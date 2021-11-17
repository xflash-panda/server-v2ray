package service

import (
	"fmt"
	"github.com/xflash-panda/server-vmess/internal/pkg/api"
	cProtocol "github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/infra/conf"
)

func buildUser(tag string, userInfo []*api.UserInfo, serverAlterID int) (users []*cProtocol.User) {
	users = make([]*cProtocol.User, len(userInfo))
	for i, user := range userInfo {
		vMessAccount := &conf.VMessAccount{
			ID:       user.UUID,
			AlterIds: uint16(serverAlterID),
			Security: "auto",
		}
		users[i] = &cProtocol.User{
			Level:   0,
			Email:   buildUserEmail(tag, user.ID, user.UUID), // Email: InboundTag|email|uid
			Account: serial.ToTypedMessage(vMessAccount.Build()),
		}
	}
	return users
}

func buildUserEmail(tag string, id int, uuid string) string {
	return fmt.Sprintf("%s|%d|%s", tag, id, uuid)
}
