package main

import (
	"common"
	"dbmgo"
	"gamelog"
	"http"
	"netConfig"
	"strconv"

	"svr_game/center"
	"svr_game/cross"
	"svr_game/logic"
	"svr_game/logic/player"
	"svr_game/sdk"
)

func main() {
	//初始化日志系统
	gamelog.InitLogger("game")
	gamelog.SetLevel(0)

	InitConf()

	//设置mongodb的服务器地址
	var id int
	cfg := netConfig.GetNetCfg("db_game", &id)
	dbmgo.Init(cfg.IP, cfg.TcpPort, cfg.SvrName)

	//开启控制台窗口，可以接受一些调试命令
	common.StartConsole()
	common.RegConsoleCmd("MakeFriends", HandCmd_MakeFriends)

	go logic.MainLoop()

	gamelog.Warn("----Game Server Start-----")
	if netConfig.CreateNetSvr("game", 1) == false {
		gamelog.Error("----Game NetSvr Failed-----")
	}
}
func HandCmd_MakeFriends(args []string) bool {
	pid1, err1 := strconv.Atoi(args[1])
	pid2, err2 := strconv.Atoi(args[2])
	if err1 != nil || err2 != nil {
		gamelog.Error("HandCmd_MakeFriends => Invalid param:%s, %s", args[1], args[2])
		return false
	}
	player1 := player.FindWithDB_PlayerId(uint32(pid1))
	player2 := player.FindWithDB_PlayerId(uint32(pid2))
	if player1 != nil && player2 != nil {
		player1.AsyncNotify(func(player *player.TPlayer) {
			player.Friend.AddFriend(player2.PlayerID, player2.Name)
		})
		player2.AsyncNotify(func(player *player.TPlayer) {
			player.Friend.AddFriend(player1.PlayerID, player1.Name)
		})
		return true
	}
	return false
}
func InitConf() {
	common.G_Csv_Map = map[string]interface{}{
		"conf_net": &netConfig.G_SvrNetCfg,
		"rpc":      &common.G_RpcCsv,
	}
	common.LoadAllCsv()

	// for k, v := range netConfig.G_SvrNetCfg {
	// 	fmt.Println(k, v)
	// }

	netConfig.RegTcpRpc(map[string]netConfig.TcpHandle{
		//! Cross
		"rpc_echo":            cross.Rpc_Echo,
		"rpc_game_battle_ack": player.Rpc_Battle_Ack,
	})
	netConfig.RegHttpHandler(map[string]netConfig.HttpHandle{
		//! SDK
		"create_recharge_order": sdk.Handle_Create_Recharge_Order,
		"sdk_recharge_success":  sdk.Handle_Recharge_Success,
	})
	netConfig.RegHttpRpc(map[string]netConfig.HttpRpc{
		//! Center
		"rpc_game_login_token": center.Rpc_Login_Token,
		//! Client
		"rpc_game_login":         player.Rpc_Player_Login,
		"rpc_game_player_create": player.Rpc_Player_Create,
	})
	netConfig.RegHttpPlayerRpc(map[string]netConfig.HttpPlayerRpc{
		//! Player
		"rpc_game_logout":             player.Rpc_Player_Logout,
		"rpc_game_heart_beat":         player.Rpc_Heart_Beat,
		"rpc_game_battle_begin":       player.Rpc_Battle_Begin,
		"rpc_game_probe_login_battle": player.Rpc_Probe_Login_Battle,
		"rpc_game_friend_list":        player.Rpc_Friend_List,
		"rpc_game_friend_apply":       player.Rpc_Friend_Apply,
		"rpc_game_friend_agree":       player.Rpc_Friend_Agree,
		"rpc_game_friend_refuse":      player.Rpc_Friend_Refuse,
		"rpc_game_friend_del":         player.Rpc_Friend_Del,
		"rpc_game_create_team":        player.Rpc_Create_Team,
		"rpc_game_exit_team":          player.Rpc_Exit_Team,
		"rpc_game_invite_friend":      player.Rpc_Invite_Friend,
		"rpc_game_agree_join_team":    player.Rpc_Agree_Join_Team,
		"rpc_game_get_team_info":      player.Rpc_Get_Team_Info,
		"rpc_game_send_team_chat":     player.Rpc_Send_Team_Chat,
		"rpc_game_get_mail":           player.Rpc_Get_Mail,
		"rpc_game_read_mail":          player.Rpc_Read_Mail,
		"rpc_game_del_mail":           player.Rpc_Del_Mail,
		"rpc_game_take_mail_item":     player.Rpc_Take_Mail_Item,
		"rpc_game_take_all_mail_item": player.Rpc_Take_All_Mail_Item,
	})
	http.G_Before_Recv_Player = player.BeforeRecvNetMsg
	http.G_After_Recv_Player = player.AfterRecvNetMsg
}
