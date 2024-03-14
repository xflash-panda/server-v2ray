package service

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	pb "github.com/xflash-panda/server-agent-proto/pkg"
	api "github.com/xflash-panda/server-client/pkg"
	cProtocol "github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/task"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/stats"
	"github.com/xtls/xray-core/proxy"
	"time"
)

type Config struct {
	FetchUsersInterval     time.Duration
	ReportTrafficsInterval time.Duration
	HeartBeatInterval      time.Duration
	Cert                   *CertConfig
	NodeID                 int
}

type Builder struct {
	instance                      *core.Instance
	config                        *Config
	nodeInfo                      *api.VMessConfig
	inboundTag                    string
	userList                      *[]api.User
	pbClient                      pb.AgentClient
	fetchUsersMonitorPeriodic     *task.Periodic
	reportTrafficsMonitorPeriodic *task.Periodic
	heartbeatMonitorPeriodic      *task.Periodic
	lastUsersHash                 string
}

// New return a builder service with default parameters.
func New(inboundTag string, instance *core.Instance, config *Config, nodeInfo *api.VMessConfig, pbClient pb.AgentClient) *Builder {
	builder := &Builder{
		inboundTag: inboundTag,
		instance:   instance,
		config:     config,
		nodeInfo:   nodeInfo,
		pbClient:   pbClient,
	}
	return builder
}

// addUsers
func (b *Builder) addUsers(users []*cProtocol.User, tag string) error {
	inboundManager := b.instance.GetFeature(inbound.ManagerType()).(inbound.Manager)
	handler, err := inboundManager.GetHandler(context.Background(), tag)
	if err != nil {
		return fmt.Errorf("no such inbound tag: %s", err)
	}
	inboundInstance, ok := handler.(proxy.GetInbound)
	if !ok {
		return fmt.Errorf("handler %s is not implement proxy.GetInbound", tag)
	}

	userManager, ok := inboundInstance.GetInbound().(proxy.UserManager)
	if !ok {
		return fmt.Errorf("handler %s is not implement proxy.UserManager", err)
	}
	for _, item := range users {
		mUser, err := item.ToMemoryUser()
		if err != nil {
			return err
		}
		err = userManager.AddUser(context.Background(), mUser)
		if err != nil {
			return err
		}
	}
	return nil
}

// addNewUser
func (b *Builder) addNewUser(userInfo []api.User) (err error) {
	users := make([]*cProtocol.User, 0)
	users = buildUser(b.inboundTag, userInfo)
	err = b.addUsers(users, b.inboundTag)
	if err != nil {
		return err
	}
	log.Infof("Added %d new users", len(userInfo))
	return nil
}

// Start implement the Start() function of the service interface
func (b *Builder) Start() error { // Update user
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	r, err := b.pbClient.Users(ctx, &pb.UsersRequest{Params: &pb.CommonParams{NodeId: int32(b.config.NodeID), NodeType: pb.NodeType_VMESS}})
	if err != nil {
		return err
	}
	userList, err := api.UnmarshalUsers(r.GetRawData())
	if err != nil {
		return err
	}
	err = b.addNewUser(*userList)
	if err != nil {
		return err
	}
	b.userList = userList
	return nil
}

func (b *Builder) StartMonitor() error {
	b.fetchUsersMonitorPeriodic = &task.Periodic{
		Interval: b.config.FetchUsersInterval,
		Execute:  b.fetchUsersMonitor,
	}
	b.reportTrafficsMonitorPeriodic = &task.Periodic{
		Interval: b.config.ReportTrafficsInterval,
		Execute:  b.reportTrafficsMonitor,
	}
	b.heartbeatMonitorPeriodic = &task.Periodic{
		Interval: b.config.HeartBeatInterval,
		Execute:  b.heartbeatMonitor,
	}

	log.Infoln("Start fetch users Monitor")
	err := b.fetchUsersMonitorPeriodic.Start()
	if err != nil {
		return fmt.Errorf("fetch users monitor periodic, start erorr:%s", err)
	}
	log.Infoln("Start report traffics Monitor")
	err = b.reportTrafficsMonitorPeriodic.Start()
	if err != nil {
		return fmt.Errorf("report traffics periodic, start erorr:%s", err)
	}

	log.Infoln("Start heartbeat task Monitor")
	err = b.heartbeatMonitorPeriodic.Start()
	if err != nil {
		return fmt.Errorf("heartbeat periodic, start erorr:%s", err)
	}
	return nil

}

// Close implement the Close() function of the service interface
func (b *Builder) Close() error {
	if b.fetchUsersMonitorPeriodic != nil {
		err := b.fetchUsersMonitorPeriodic.Close()
		if err != nil {
			return fmt.Errorf("fetch users monitor periodic close failed: %s", err)
		}
	}

	if b.reportTrafficsMonitorPeriodic != nil {
		err := b.reportTrafficsMonitorPeriodic.Close()
		if err != nil {
			return fmt.Errorf("report traffics periodic close failed: %s", err)
		}
	}

	if err := b.heartbeatMonitorPeriodic.Close(); err != nil {
		log.Warn("heartbeat task close error: ", err)
	}
	return nil
}

// getTraffic
func (b *Builder) getTraffic(email string) (up int64, down int64, count int64) {
	upName := "user>>>" + email + ">>>traffic>>>uplink"
	downName := "user>>>" + email + ">>>traffic>>>downlink"
	countName := "user>>>" + email + ">>>request>>>count"
	statsManager := b.instance.GetFeature(stats.ManagerType()).(stats.Manager)
	upCounter := statsManager.GetCounter(upName)
	downCounter := statsManager.GetCounter(downName)
	countCounter := statsManager.GetCounter(countName)
	if upCounter != nil {
		up = upCounter.Value()
		upCounter.Set(0)
	}
	if downCounter != nil {
		down = downCounter.Value()
		downCounter.Set(0)
	}
	if countCounter != nil {
		count = countCounter.Value()
		countCounter.Set(0)
	}

	return up, down, count

}

// removeUsers
func (b *Builder) removeUsers(users []string, tag string) error {
	inboundManager := b.instance.GetFeature(inbound.ManagerType()).(inbound.Manager)
	handler, err := inboundManager.GetHandler(context.Background(), tag)
	if err != nil {
		return fmt.Errorf("no such inbound tag: %s", err)
	}
	inboundInstance, ok := handler.(proxy.GetInbound)
	if !ok {
		return fmt.Errorf("handler %s is not implement proxy.GetInbound", tag)
	}

	userManager, ok := inboundInstance.GetInbound().(proxy.UserManager)
	if !ok {
		return fmt.Errorf("handler %s is not implement proxy.UserManager", err)
	}
	for _, email := range users {
		err = userManager.RemoveUser(context.Background(), email)
		if err != nil {
			return err
		}
	}
	return nil
}

// nodeInfoMonitor
func (b *Builder) reportTrafficsMonitor() (err error) {
	// Get User traffic
	userTraffics := make([]*api.UserTraffic, 0)
	var i int
	var trafficStats api.TrafficStats
	trafficStats.UserIds = make([]int, 0)
	trafficStats.UserRequests = make(map[int]int)
	for _, user := range *b.userList {
		up, down, count := b.getTraffic(buildUserEmail(b.inboundTag, user.ID, user.UUID))
		if up > 0 || down > 0 || count > 0 {
			userTraffics = append(userTraffics, &api.UserTraffic{
				UID:      user.ID,
				Upload:   uint64(up),
				Download: uint64(down),
				Count:    uint64(count),
			})
		}

		if count > 0 {
			trafficStats.Count++
			trafficStats.Requests += int(count)
			trafficStats.UserIds = append(trafficStats.UserIds, user.ID)
			trafficStats.UserRequests[user.ID] = int(count)
		}
		i++
	}
	log.Infof("%d user traffic needs to be reported", len(userTraffics))
	trafficsRawData, err := api.MarshalTraffics(userTraffics)
	if err != nil {
		log.Errorln(err)
		return nil
	}

	statsRawData, err := api.MarshalTrafficStats(&trafficStats)
	if err != nil {
		log.Errorln(err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	_, err = b.pbClient.Submit(ctx, &pb.SubmitRequest{Params: &pb.CommonParams{NodeId: int32(b.config.NodeID), NodeType: pb.NodeType_VMESS}, RawData: trafficsRawData, RawStats: statsRawData})
	if err != nil {
		log.Errorln(err)
		return nil
		//return err
	}
	return nil
}

func (b *Builder) fetchUsersMonitor() (err error) {
	// Update User
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	r, err := b.pbClient.Users(ctx, &pb.UsersRequest{Params: &pb.CommonParams{NodeId: int32(b.config.NodeID), NodeType: pb.NodeType_VMESS}, Hash: &b.lastUsersHash})
	if err != nil {
		log.Errorln(err)
		return nil
	}

	if r.GetStatus() == pb.ChangeStatus_NOT_CHANGED {
		log.Infoln("users not modified")
		return nil
	}
	b.lastUsersHash = r.GetHash()
	newUserList, err := api.UnmarshalUsers(r.GetRawData())
	if err != nil {
		log.Errorln(err)
		return nil
	}

	deleted, added := b.compareUserList(newUserList)
	if len(deleted) > 0 {
		deletedEmail := make([]string, len(deleted))
		for i, u := range deleted {
			deletedEmail[i] = buildUserEmail(b.inboundTag, u.ID, u.UUID)
		}
		err := b.removeUsers(deletedEmail, b.inboundTag)
		if err != nil {
			log.Errorln(err)
			return nil
		}
	}
	if len(added) > 0 {
		err = b.addNewUser(added)
		if err != nil {
			log.Errorln(err)
			return nil
		}

	}
	log.Infof("%d user deleted, %d user added", len(deleted), len(added))
	b.userList = newUserList
	return nil
}

func (b *Builder) heartbeatMonitor() error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	log.Infoln("heartbeat...")
	_, err := b.pbClient.Heartbeat(ctx, &pb.HeartbeatRequest{Params: &pb.CommonParams{NodeId: int32(b.config.NodeID), NodeType: pb.NodeType_VMESS}})
	if err != nil {
		log.Errorln(err)
		return nil
	}
	return nil
}

// compareUserList
func (b *Builder) compareUserList(newUsers *[]api.User) (deleted, added []api.User) {
	// 使用map来标记旧用户列表中的每个用户
	userMap := make(map[api.User]bool)

	// 标记旧用户列表中所有用户为已删除（暂时）
	for _, user := range *b.userList {
		userMap[user] = true
	}

	// 遍历新用户列表
	for _, newUser := range *newUsers {
		if userMap[newUser] {
			// 如果当前用户在旧列表中，标记为未删除（即用户仍在列表中）
			userMap[newUser] = false
		} else {
			// 如果用户不在旧列表中，那么它是一个新增用户
			added = append(added, newUser)
		}
	}

	// 任何在userMap中仍标记为true的用户都是被删除的
	for user, isDeleted := range userMap {
		if isDeleted {
			deleted = append(deleted, user)
		}
	}

	return deleted, added
}
