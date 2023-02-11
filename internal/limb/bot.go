package limb

import (
	"bytes"
	"fmt"
	"time"

	"github.com/duo/octopus-wechat-web/internal/common"

	"github.com/eatmoreapple/openwechat"
	"github.com/skip2/go-qrcode"

	log "github.com/sirupsen/logrus"
)

type Bot struct {
	config *common.Configure

	client *openwechat.Bot
	self   *openwechat.Self

	pushFunc func(*common.OctopusEvent)

	stopping bool
	stopSync chan struct{}
}

func (b *Bot) Login() {
	b.client.MessageHandler = b.processWechatMessage

	b.client.UUIDCallback = func(uuid string) {
		q, _ := qrcode.New("https://login.weixin.qq.com/l/"+uuid, qrcode.Low)
		fmt.Println(q.ToString(true))
	}

	/*
		reloadStorage := openwechat.NewFileHotReloadStorage("storage.json")

		err := b.client.HotLogin(reloadStorage)
		if err != nil {
			if err = b.client.Login(); err != nil {
				log.Fatalf("Failed to login: %v", err)
			}
		}
	*/
	if err := b.client.Login(); err != nil {
		log.Fatalf("Failed to login: %v", err)
	}

	self, err := b.client.GetCurrentUser()
	if err != nil {
		log.Fatal(err)
	}
	if self.ID() == "" {
		log.Fatalln("Failed to get self id")
	}
	b.self = self
}

func (b *Bot) Start() {
	log.Infoln("Bot started")

	go func() {
		b.client.Block()
	}()

	go func() {
		time.Sleep(b.config.Service.SyncDelay)
		go b.sync()

		clock := time.NewTicker(b.config.Service.SyncInterval)
		defer func() {
			log.Infoln("LimbService sync stopped")
			clock.Stop()
		}()
		log.Infof("Syncing LimbService every %s", b.config.Service.SyncInterval)
		for {
			select {
			case <-clock.C:
				go b.sync()
			case <-b.stopSync:
				return
			}
		}
	}()
}

func (b *Bot) Stop() {
	log.Infoln("Bot stopping")

	b.stopping = true

	select {
	case b.stopSync <- struct{}{}:
	default:
	}

	b.client.Logout()
}

func NewBot(config *common.Configure, pushFunc func(*common.OctopusEvent)) *Bot {
	return &Bot{
		config:   config,
		client:   openwechat.DefaultBot(openwechat.Desktop),
		pushFunc: pushFunc,
		stopSync: make(chan struct{}),
	}
}

func (b *Bot) sync() {
	event := b.generateEvent("sync", time.Now().UnixMilli())

	chats := []*common.Chat{}

	chats = append(chats, &common.Chat{
		ID:    b.self.ID(),
		Type:  "private",
		Title: b.self.NickName,
	})

	if friends, err := b.self.Friends(); err != nil {
		log.Warnf("Failed to get friend list: %v", err)
	} else {
		for _, f := range friends {
			log.Warnf("Friend: %s %s", f.ID(), f.UserName)
			title := f.ID()
			if f.NickName != "" {
				title = f.NickName
			}
			if f.RemarkName != "" {
				title = f.RemarkName
			}
			chats = append(chats, &common.Chat{
				ID:    f.ID(),
				Type:  "private",
				Title: title,
			})
		}
	}

	if groups, err := b.self.Groups(); err != nil {
		log.Warnf("Failed to get group list: %v", err)
	} else {
		for _, g := range groups {
			log.Warnf("Group: %s %s", g.ID(), g.UserName)
			title := g.ID()
			if g.NickName != "" {
				title = g.NickName
			}
			chats = append(chats, &common.Chat{
				ID:    g.ID(),
				Type:  "group",
				Title: title,
			})
		}
	}

	event.Type = common.EventSync
	event.Data = chats

	b.pushFunc(event)
}

// process events from master
func (b *Bot) processOcotopusEvent(event *common.OctopusEvent) (*common.OctopusEvent, error) {
	var sent *openwechat.SentMessage
	var err error
	target := event.Chat.ID

	var friend *openwechat.Friend
	if target == openwechat.FileHelper {
		friend = b.self.FileHelper()
	} else {
		members, err := b.self.Members()
		if err != nil {
			return nil, err
		}
		users := members.Search(1, func(user *openwechat.User) bool { return user.ID() == target })
		user := users.First()
		if user == nil {
			return nil, fmt.Errorf("chat %s not found", target)
		} else {
			friend = &openwechat.Friend{User: user}
		}
	}

	switch event.Type {
	case common.EventText:
		sent, err = b.self.SendTextToFriend(friend, event.Content)
	case common.EventPhoto:
		blob := event.Data.([]*common.BlobData)[0]
		sent, err = b.self.SendImageToFriend(friend, bytes.NewReader(blob.Binary))
	case common.EventVideo:
		blob := event.Data.(*common.BlobData)
		sent, err = b.self.SendVideoToFriend(friend, bytes.NewReader(blob.Binary))
	case common.EventFile:
		blob := event.Data.(*common.BlobData)
		sent, err = b.self.SendFileToFriend(friend, bytes.NewReader(blob.Binary))
	default:
		err = fmt.Errorf("event type not support: %s", event.Type)
	}

	if err != nil {
		return nil, err
	}

	return &common.OctopusEvent{
		ID:        sent.MsgId,
		Timestamp: time.Now().Unix(),
	}, err
}

// process WeChat message
func (b *Bot) processWechatMessage(msg *openwechat.Message) {
	if msg.MsgType == 51 {
		return
	}

	event := b.generateEvent(msg.MsgId, msg.CreateTime)

	var blob *common.BlobData
	var dataErr error
	if msg.HasFile() {
		blob, dataErr = download(msg.GetFile())
	}

	switch msg.MsgType {
	case openwechat.MsgTypeText:
		event.Type = common.EventText
		event.Content = msg.Content
	case openwechat.MsgTypeImage:
		if dataErr != nil {
			event.Type = common.EventText
			event.Content = "[图片下载失败]"
		} else {
			event.Type = common.EventPhoto
			event.Data = []*common.BlobData{blob}
		}
	case openwechat.MsgTypeEmoticon:
		if dataErr != nil {
			event.Type = common.EventText
			event.Content = "[表情下载失败]"
		} else {
			event.Type = common.EventPhoto
			event.Data = []*common.BlobData{blob}
		}
	case openwechat.MsgTypeVoice:
		if dataErr != nil {
			event.Type = common.EventText
			event.Content = "[语音下载失败]"
		} else {
			event.Type = common.EventAudio
			event.Data = blob
		}
	case openwechat.MsgTypeShareCard:
		if card, err := msg.Card(); err != nil {
			log.Warnf("Failed to parse card message: %v", err)
		} else {
			event.Type = common.EventApp
			event.Data = &common.AppData{
				Title:       "",
				Description: card.NickName,
				Source:      card.NickName,
				URL:         card.BigHeadImgUrl,
			}
		}
	case openwechat.MsgTypeVideo:
		if dataErr != nil {
			event.Type = common.EventText
			event.Content = "[视频下载失败]"
		} else {
			event.Type = common.EventVideo
			event.Data = blob
		}
	case openwechat.MsgTypeLocation:
		event.Type = common.EventText
		event.Content = msg.Content
	case openwechat.MsgTypeVoip, openwechat.MsgTypeVoipNotify, openwechat.MsgTypeVoipInvite:
		event.Type = common.EventVoIP
		event.Content = msg.MsgType.String()
	case openwechat.MsgTypeSys:
		event.Type = common.EventText
		event.Content = msg.Content
	case openwechat.MsgTypeRecalled:
		if revokeMsg, err := msg.RevokeMsg(); err != nil {
			log.Warnf("Failed to parse revoke message: %v", err)
		} else {
			event.Reply = &common.ReplyInfo{
				ID: common.Itoa(revokeMsg.RevokeMsg.MsgId),
			}
			event.Type = common.EventRevoke
			event.Content = revokeMsg.RevokeMsg.ReplaceMsg
		}
	case openwechat.MsgTypeApp:
		switch msg.AppMsgType {
		case openwechat.AppMsgTypeAttach:
			if dataErr != nil {
				event.Type = common.EventText
				event.Content = "[文件下载失败]"
			} else {
				event.Type = common.EventFile
				event.Data = blob
			}
		case openwechat.AppMsgTypeEmoji:
			blob := downloadSticker(b, msg.Content)
			if blob != nil {
				event.Type = common.EventPhoto
				event.Data = []*common.BlobData{blob}
				event.Content = ""
			} else {
				event.Type = common.EventText
				event.Content = "[表情下载失败]"
			}
		case 1, 51, 63:
			app := parseApp(b, msg.Content, msg.AppMsgType)
			if app != nil {
				event.Type = common.EventApp
				event.Data = app
			} else {
				event.Content = "[应用解析失败]"
			}
		default:
			if media, err := msg.MediaData(); err != nil {
				log.Warnf("Failed to parse media message: %v", err)
				return
			} else {
				event.Type = common.EventApp
				event.Data = &common.AppData{
					Title:       media.AppMsg.Title,
					Description: media.AppMsg.Des,
					Source:      media.AppInfo.AppName,
					URL:         media.AppMsg.URL,
				}
			}
		}
	default:
		log.Warnf("message type not support: %s", msg.MsgType)
		return
	}

	if msg.IsSendBySelf() {
		event.From = common.User{
			ID:       b.self.ID(),
			Username: b.self.NickName,
			Remark:   b.self.NickName,
		}

		members, err := b.self.Members()
		if err != nil {
			log.Warnf("Failed to get members")
			return
		}
		users := members.SearchByUserName(1, msg.ToUserName)
		user := users.First()
		if user == nil {
			log.Warnf("Failed to get chat: %s", msg.ToUserName)
			return
		}
		event.Chat = common.Chat{
			Type:  "private",
			ID:    user.ID(),
			Title: user.NickName,
		}
		if msg.IsSendByGroup() {
			event.Chat.Type = "group"
		}
	} else {
		sender, err := msg.Sender()
		chat := sender
		if msg.IsSendByGroup() {
			sender, err = msg.SenderInGroup()
		}
		if err != nil {
			log.Warnf("Failed to get message sender: %v", err)
			return
		}
		remark := sender.RemarkName
		if sender.DisplayName != "" {
			remark = sender.DisplayName
		}
		event.From = common.User{
			ID:       sender.ID(),
			Username: sender.NickName,
			Remark:   remark,
		}

		if msg.IsSendByGroup() {
			event.Chat = common.Chat{
				Type:  "group",
				ID:    chat.ID(),
				Title: chat.NickName,
			}
		} else {
			event.Chat = common.Chat{
				Type:  "private",
				ID:    chat.ID(),
				Title: remark,
			}
		}
	}

	log.Debugf("Push event: %+v", event)

	b.pushFunc(event)
}

func (b *Bot) generateEvent(id string, ts int64) *common.OctopusEvent {
	return &common.OctopusEvent{
		Vendor:    b.getVendor(),
		ID:        id,
		Timestamp: ts,
	}
}

func (b *Bot) getVendor() common.Vendor {

	return common.Vendor{
		Type: "wechat",
		UID:  b.self.User.ID(),
	}
}
