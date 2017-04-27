/***********************************************************************
* @ 玩家数据
* @ brief
	1、数据散列模块化，按业务区分成块，各自独立处理，如：TMailMoudle
	2、可调用DB【同步读单个模块】，编辑后再【异步写】
	3、本文件的数据库操作接口，都是【同步的】

* @ 访问离线玩家
	1、用什么取什么，读出一块数据编辑完后写回，尽量少载入整个玩家结构体
	2、设想把TPlayer里的数据块部分全定义为指针，各模块分别做个缓存表(online list、offline list)
	3、但觉得有些设计冗余，缓存这种事情，应该交给DBCache系统做，业务层不该负责这事儿

* @ 自动写数据库
	1、借助ServicePatch，十五分钟全写一遍在线玩家，重要数据才手动异步写dbmgo.InsertToDB
	2、关服，须先踢所有玩家下线，触发Logou流程写库，再才能关闭进程

* @ author zhoumf
* @ date 2017-4-22
***********************************************************************/
package player

import (
	"common"
	"dbmgo"
)

var (
	G_auto_write_db = common.NewServicePatch(_WritePlayerToDB, 15*60*1000)
)

type PlayerMoudle interface {
	InitAndInsert(*TPlayer)
	LoadFromDB(*TPlayer)
	WriteToDB()
	OnLogin()
	OnLogout()
}
type TPlayer struct {
	//db data
	TPlayerBase
	Mail   TMailMoudle
	Friend TFriendMoudle
	//temp data
	moudles []PlayerMoudle
	isOnlie bool
}
type TPlayerBase struct {
	PlayerID   uint32 `bson:"_id"`
	AccountID  uint32
	Name       string
	LoginTime  int64
	LogoutTime int64
}

func NewPlayer() *TPlayer {
	player := new(TPlayer)
	//! regist
	player.moudles = []PlayerMoudle{
		&player.Mail,
	}
	return player
}
func NewPlayerInDB(accountId uint32, id uint32, name string) *TPlayer {
	player := NewPlayer()
	player.AccountID = accountId
	player.PlayerID = id
	player.Name = name
	if dbmgo.InsertSync("Player", &player.TPlayerBase) {
		for _, v := range player.moudles {
			v.InitAndInsert(player)
		}
		return player
	}
	return nil
}
func LoadPlayerFromDB(key string, val uint32) *TPlayer {
	player := NewPlayer()
	if dbmgo.Find("Player", key, val, &player.TPlayerBase) {
		for _, v := range player.moudles {
			v.LoadFromDB(player)
		}
		return player
	}
	return nil
}
func (self *TPlayer) WriteAllToDB() {
	if dbmgo.UpdateSync("Player", self.PlayerID, &self.TPlayerBase) {
		for _, v := range self.moudles {
			v.WriteToDB()
		}
	}
}
func (self *TPlayer) OnLogin() {
	self.isOnlie = true
	for _, v := range self.moudles {
		v.OnLogin()
	}
	G_auto_write_db.Register(self)
}
func (self *TPlayer) OnLogout() {
	self.isOnlie = false
	for _, v := range self.moudles {
		v.OnLogout()
	}
	DelPlayerCache(self.PlayerID)
	G_auto_write_db.UnRegister(self)
}
func _WritePlayerToDB(ptr interface{}) {
	if player, ok := ptr.(TPlayer); ok {
		player.WriteAllToDB()
	}
}
