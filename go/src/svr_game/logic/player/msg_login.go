package player

import (
	"common"
)

func Rpc_Player_Login(req, ack *common.ByteBuffer) {
	//req: accountId, loginKey(账号服生成的登录验证码)
	//ack: playerId, data
	accountId := req.ReadUInt32()

	if player := FindWithDB_AccountId(accountId); player != nil {

	} else {
		//notify client to create new player
	}
}
func Rpc_Player_Logout(req, ack *common.ByteBuffer) {
	//req: playerId
	playerId := req.ReadUInt32()
	DelPlayerCache(playerId)
}
func Rpc_Player_Create(req, ack *common.ByteBuffer) {
	//req: accountId, loginKey, playerName
	//ack: playerId
	accountId := req.ReadUInt32()
	playerName := req.ReadString()
	if player := AddNewPlayer(accountId, playerName); player != nil {
		ack.WriteUInt32(player.Base.PlayerID)
	}
}
