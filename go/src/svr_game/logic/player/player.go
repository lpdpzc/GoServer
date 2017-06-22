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

* @ 玩家之间互改数据【多线程架构】
	1、禁止直接操作对方内存

	2、异步间接改别人的数据
			*、提供统一接口，将写操作发送到目标所在线程，让目标自己改写
			*、因为读别人数据是直接拿内存，此方式可能带来时序Bug【异步写在读之前，但读到旧数据】
			*、比如：异步扣别人100块，又立即读，可能他还是没钱

	3、分别加读写锁【多读少写用RWMutex，写也多的用Mutex】
			*、会被其他人改的数据块，性质上同全局数据类似，多读少写的
			*、读写锁封装接口，谁都不允许直接访问
			*、比异步方式(可能读到旧值)安全，但要写好锁代码【屏蔽所有竞态条件、无死锁】可不是件容易事~_~

* @ author zhoumf
* @ date 2017-4-22
***********************************************************************/
package player

import (
	"common"
	"dbmgo"
	"gamelog"
	"sync/atomic"
	"time"
)

var (
	G_Auto_Write_DB = common.NewServicePatch(_WritePlayerToDB, 15*60*1000)
)

const (
	Idle_Max_Second       = 10
	Reconnect_Wait_Second = 30
)

type MoudleInterface interface {
	InitAndInsert(*TPlayer)
	LoadFromDB(*TPlayer)
	WriteToDB()
	OnLogin()
	OnLogout()
}
type TPlayerBase struct {
	PlayerID   uint32 `bson:"_id"`
	AccountID  uint32
	Name       string
	LoginTime  int64
	LogoutTime int64
}
type TPlayer struct {
	//temp data
	moudles []MoudleInterface
	askchan chan func(*TPlayer)
	isOnlie bool
	idleSec uint32
	//db data
	TPlayerBase
	Mail   TMailMoudle
	Friend TFriendMoudle
	Chat   TChatMoudle
	Battle TBattleMoudle
}

func _NewPlayer() *TPlayer {
	player := new(TPlayer)
	//! regist
	player.moudles = []MoudleInterface{
		&player.Mail,
		&player.Friend,
		&player.Chat,
		&player.Battle,
	}
	player.askchan = make(chan func(*TPlayer), 128)
	return player
}
func _NewPlayerInDB(accountId uint32, id uint32, name string) *TPlayer {
	player := _NewPlayer()
	if dbmgo.Find("Account", "name", name, player) {
		return nil
	}
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
	player := _NewPlayer()
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
func (self *TPlayer) Login() {
	self.isOnlie = true
	atomic.SwapUint32(&self.idleSec, 0)
	for _, v := range self.moudles {
		v.OnLogin()
	}
	G_Auto_Write_DB.Register(self)
}
func (self *TPlayer) Logout() {
	self.isOnlie = false
	for _, v := range self.moudles {
		v.OnLogout()
	}
	G_Auto_Write_DB.UnRegister(self)

	// 延时30s后再删，提升重连效率
	time.AfterFunc(Reconnect_Wait_Second*time.Second, func() { //Notice:AfterFunc是在另一线程执行，所以调的函数须是线程安全的
		ptr := self //闭包，引用指针，直接self.isOnlie是值传递
		if !ptr.isOnlie {
			gamelog.Info("Pid(%d) Delete", ptr.PlayerID)
			go ptr.WriteAllToDB()
			DelPlayerCache(ptr.PlayerID)
		}
	})
}
func _WritePlayerToDB(ptr interface{}) {
	if player, ok := ptr.(*TPlayer); ok {
		player.WriteAllToDB()
	}
}
func CheckAFK() { //测试代码：临时全服遍历
	for _, v := range g_player_cache {
		if v.isOnlie {
			_CheckAFK(v)
		}
	}
}
func _CheckAFK(ptr interface{}) {
	if player, ok := ptr.(*TPlayer); ok && player.isOnlie {
		atomic.AddUint32(&player.idleSec, 1)
		if player.idleSec > Idle_Max_Second {
			gamelog.Info("Pid(%d) AFK", player.PlayerID)
			player.Logout()
		}
	}
}

//////////////////////////////////////////////////////////////////////
//! for other player write my data
func AsyncNotifyPlayer(pid uint32, handler func(*TPlayer)) {
	if player := _FindInCache(pid); player != nil {
		player.AsyncNotify(handler)
	}
}
func (self *TPlayer) AsyncNotify(handler func(*TPlayer)) {
	if self.isOnlie {
		select {
		case self.askchan <- handler:
		default:
			gamelog.Warn("Player askChan is full !!!")
			return
		}
	} else { //TODO:zhouf: 如何安全方便的修改离线玩家数据

		//准备将离线的操作转给mainloop，这样所有离线玩家就都在一个chan里处理了
		//要是中途玩家上线，mainloop的chan里还有他的操作没处理完怎么整！？囧~
		//mainloop设计成map<pid, chan>，玩家上线时，检测自己的chan有效否，等它处理完？

		//gen_server
		//将某个独立模块的所有操作扔进gen_server，外界只读(有滞后性)
		//会加大代码量，每个操作都得转一次到chan
		//【Notice】可能gen_server里还有修改操作，且玩家已下线，会重新读到内存，此时修改完毕后须及时入库

		//设计统一的接口，编辑离线数据，也很麻烦呐		//准备将离线的操作转给mainloop，这样所有离线玩家就都在一个chan里处理了
		//要是中途玩家上线，mainloop的chan里还有他的操作没处理完怎么整！？囧~
		//mainloop设计成map<pid, chan>，玩家上线时，检测自己的chan有效否，等它处理完？

		//gen_server
		//将某个独立模块的所有操作扔进gen_server，外界只读(有滞后性)
		//会加大代码量，每个操作都得转一次到chan
		//【Notice】可能gen_server里还有修改操作，且玩家已下线，会重新读到内存，此时修改完毕后须及时入库

		//设计统一的接口，编辑离线数据，也很麻烦呐
	}
}
func (self *TPlayer) _HandleAsyncNotify() {
	for {
		select {
		case handler := <-self.askchan:
			handler(self)
		default:
			return
		}
	}
}

//////////////////////////////////////////////////////////////////////
//! 访问玩家部分数据，包括离线的
func GetPlayerBaseData(pid uint32) *TPlayerBase {
	if player := _FindInCache(pid); player != nil {
		return &player.TPlayerBase
	} else {
		ptr := new(TPlayerBase)
		if dbmgo.Find("Player", "_id", pid, ptr) {
			return ptr
		}
		return nil
	}
}
