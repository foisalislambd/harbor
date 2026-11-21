package telegram

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/contrib/middleware/floodwait"
)

type gotdEngine struct{}

type memorySession struct {
	mu   sync.Mutex
	data []byte
}

func (m *memorySession) LoadSession(context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.data) == 0 {
		return nil, session.ErrNotFound
	}
	return append([]byte(nil), m.data...), nil
}

func (m *memorySession) StoreSession(_ context.Context, data []byte) error {
	m.mu.Lock()
	m.data = append([]byte(nil), data...)
	m.mu.Unlock()
	return nil
}

func (m *memorySession) snapshot() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.data...)
}

func (g gotdEngine) run(ctx context.Context, apiID int, cred *credentials, fn func(context.Context, *telegram.Client) error) error {
	storage := &memorySession{data: append([]byte(nil), cred.Session...)}
	err := g.runStored(ctx, apiID, cred.APIHash, storage, false, fn)
	if snap := storage.snapshot(); len(snap) > 0 {
		cred.Session = snap
	}
	if errors.Is(err, ErrPasswordNeeded) {
		return ErrPasswordNeeded
	}
	if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return redact(ctx.Err(), *cred)
	}
	return redact(explain(err), *cred)
}

// runStored connects and then calls fn. Reaching Telegram is limited to authBudget
// so a blocked datacenter cannot hold an HTTP request until a proxy times out.
// After fn starts, ctx controls the rest of the call.
func (gotdEngine) runStored(ctx context.Context, apiID int, apiHash string, storage *memorySession, authCall bool, fn func(context.Context, *telegram.Client) error) error {
	attempt, stop := context.WithCancel(ctx)
	defer stop()
	started := make(chan struct{})
	go func() {
		timer := time.NewTimer(authBudget)
		defer timer.Stop()
		select {
		case <-timer.C:
			select {
			case <-started:
			default:
				stop()
			}
		case <-attempt.Done():
		case <-started:
		}
	}()
	maxWait := 2 * time.Minute
	retries := 5
	if authCall {
		// A long FLOOD_WAIT must be shown to the user, not slept through.
		maxWait = 5 * time.Second
		retries = 1
	}
	waiter := floodwait.NewWaiter().WithMaxWait(maxWait).WithMaxRetries(retries)
	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage:   storage,
		Middlewares:      []telegram.Middleware{waiter},
		MaxRetries:       3,
		DialTimeout:      10 * time.Second,
		ExchangeTimeout:  10 * time.Second,
		MigrationTimeout: 15 * time.Second,
		NoUpdates:        true,
		ReconnectionBackoff: func() backoff.BackOff {
			b := backoff.NewExponentialBackOff()
			b.InitialInterval = 200 * time.Millisecond
			b.MaxInterval = 2 * time.Second
			b.MaxElapsedTime = authBudget
			return b
		},
		Device: telegram.DeviceConfig{
			DeviceModel:   "Harbor",
			SystemVersion: "server",
			AppVersion:    "harbor",
		},
	})
	err := waiter.Run(attempt, func(ctx context.Context) error {
		return client.Run(ctx, func(ctx context.Context) error {
			close(started)
			return fn(ctx, client)
		})
	})
	if ctx.Err() == nil && errors.Is(err, context.Canceled) {
		return errTelegramUnreachable
	}
	return err
}

func (g gotdEngine) SendCode(ctx context.Context, apiID int, cred *credentials, phone string) (string, error) {
	var delivery string
	err := g.authCall(ctx, apiID, phone, cred, true, func(ctx context.Context, client *telegram.Client) error {
		sent, err := sendCode(ctx, client, phone)
		if err != nil {
			return err
		}
		switch code := sent.(type) {
		case *tg.AuthSentCode:
			cred.PhoneCodeHash = code.PhoneCodeHash
			cred.Authorized = false
			delivery = deliveryName(code.Type)
			return nil
		case *tg.AuthSentCodeSuccess:
			if authz, ok := code.Authorization.(*tg.AuthAuthorization); ok {
				cred.User = userLabel(authz.User)
				cred.Premium = userPremium(authz.User)
				cred.Authorized = true
			}
			delivery = "already"
			return nil
		default:
			return errors.New("this number needs an extra step in the official Telegram app before Harbor can sign in")
		}
	})
	return delivery, err
}

func sendCode(ctx context.Context, client *telegram.Client, phone string) (tg.AuthSentCodeClass, error) {
	send := func() (tg.AuthSentCodeClass, error) {
		return client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
	}
	sent, err := send()
	if dc, ok := migrateDC(err); ok {
		if mErr := client.MigrateTo(ctx, dc); mErr != nil {
			return nil, err
		}
		sent, err = send()
	}
	return sent, err
}

func deliveryName(kind tg.AuthSentCodeTypeClass) string {
	switch kind.(type) {
	case *tg.AuthSentCodeTypeApp:
		return "app"
	case *tg.AuthSentCodeTypeSMS, *tg.AuthSentCodeTypeFragmentSMS, *tg.AuthSentCodeTypeFirebaseSMS:
		return "sms"
	case *tg.AuthSentCodeTypeCall, *tg.AuthSentCodeTypeFlashCall, *tg.AuthSentCodeTypeMissedCall:
		return "call"
	default:
		return "telegram"
	}
}

func (g gotdEngine) SignIn(ctx context.Context, apiID int, cred *credentials, phone, code string) (string, error) {
	var name string
	err := g.authCall(ctx, apiID, phone, cred, false, func(ctx context.Context, client *telegram.Client) error {
		sign := func() (*tg.AuthAuthorization, error) {
			return client.Auth().SignIn(ctx, phone, code, cred.PhoneCodeHash)
		}
		authz, err := sign()
		if dc, ok := migrateDC(err); ok {
			if mErr := client.MigrateTo(ctx, dc); mErr != nil {
				return err
			}
			authz, err = sign()
		}
		if errors.Is(err, auth.ErrPasswordAuthNeeded) {
			return ErrPasswordNeeded
		}
		if err != nil {
			return err
		}
		name = userLabel(authz.User)
		cred.Premium = userPremium(authz.User)
		return nil
	})
	if err == nil {
		authSessions.drop(loginKey(apiID, phone, cred.APIHash))
	}
	if errors.Is(err, ErrPasswordNeeded) {
		return "", ErrPasswordNeeded
	}
	return name, err
}

func (g gotdEngine) Password(ctx context.Context, apiID int, cred *credentials, phone, password string) (string, error) {
	var name string
	err := g.authCall(ctx, apiID, phone, cred, false, func(ctx context.Context, client *telegram.Client) error {
		authz, err := client.Auth().Password(ctx, password)
		if err != nil {
			return err
		}
		name = userLabel(authz.User)
		cred.Premium = userPremium(authz.User)
		return nil
	})
	if err == nil {
		authSessions.drop(loginKey(apiID, phone, cred.APIHash))
	}
	return name, err
}

func (g gotdEngine) LogOut(ctx context.Context, apiID int, cred *credentials, phone string) error {
	key := loginKey(apiID, phone, cred.APIHash)
	if authSessions.get(key) != nil {
		err := g.authCall(ctx, apiID, phone, cred, false, func(ctx context.Context, client *telegram.Client) error {
			if _, err := client.API().AuthLogOut(ctx); err != nil && !auth.IsUnauthorized(err) {
				return err
			}
			return nil
		})
		authSessions.drop(key)
		cred.Session = nil
		return err
	}
	if len(cred.Session) == 0 {
		return nil
	}
	err := g.run(ctx, apiID, cred, func(ctx context.Context, client *telegram.Client) error {
		if _, err := client.API().AuthLogOut(ctx); err != nil && !auth.IsUnauthorized(err) {
			return err
		}
		return nil
	})
	cred.Session = nil
	return err
}

func (g gotdEngine) Self(ctx context.Context, apiID int, cred *credentials) (string, bool, error) {
	var name string
	var premium bool
	err := g.run(ctx, apiID, cred, func(ctx context.Context, client *telegram.Client) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized || status.User == nil {
			return errors.New("sign in to Telegram from Destinations first")
		}
		name = userLabel(status.User)
		premium = status.User.GetPremium()
		return nil
	})
	return name, premium, err
}

func (g gotdEngine) Resolve(ctx context.Context, apiID int, cred *credentials, target string) (storedPeer, error) {
	var peer storedPeer
	err := g.run(ctx, apiID, cred, func(ctx context.Context, client *telegram.Client) error {
		found, err := resolvePeer(ctx, client.API(), target)
		if err != nil {
			return err
		}
		peer = found
		return nil
	})
	return peer, err
}

func (g gotdEngine) Upload(ctx context.Context, apiID int, cred *credentials, peer storedPeer, topic int, file *os.File, offset, n int64, filename, caption string) (int, error) {
	var id int
	err := g.run(ctx, apiID, cred, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		randomID, err := randomID()
		if err != nil {
			return err
		}
		var media tg.InputMediaClass
		if n > 0 {
			input, err := uploader.NewUploader(api).WithThreads(4).Upload(ctx, uploader.NewUpload(filename, io.NewSectionReader(file, offset, n), n))
			if err != nil {
				return err
			}
			media = &tg.InputMediaUploadedDocument{
				File:     input,
				MimeType: "application/octet-stream",
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeFilename{FileName: filename},
				},
			}
		}
		var updates tg.UpdatesClass
		if media == nil {
			req := &tg.MessagesSendMessageRequest{Peer: inputPeer(peer), Message: caption, RandomID: randomID}
			if topic > 0 {
				req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: topic})
			}
			updates, err = api.MessagesSendMessage(ctx, req)
		} else {
			req := &tg.MessagesSendMediaRequest{Peer: inputPeer(peer), Media: media, Message: caption, RandomID: randomID}
			if topic > 0 {
				req.SetReplyTo(&tg.InputReplyToMessage{ReplyToMsgID: topic})
			}
			updates, err = api.MessagesSendMedia(ctx, req)
		}
		if err != nil {
			return err
		}
		id, err = sentMessageID(updates, randomID)
		return err
	})
	return id, err
}

func (g gotdEngine) Delete(ctx context.Context, apiID int, cred *credentials, ref objectRef) error {
	if len(ref.Messages) == 0 {
		return nil
	}
	return g.run(ctx, apiID, cred, func(ctx context.Context, client *telegram.Client) error {
		api := client.API()
		var err error
		if ref.Kind == "channel" {
			_, err = api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
				Channel: &tg.InputChannel{ChannelID: ref.ID, AccessHash: ref.Access},
				ID:      ref.Messages,
			})
		} else {
			_, err = api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{Revoke: true, ID: ref.Messages})
		}
		if err == nil || tgerr.Is(err, "MESSAGE_ID_INVALID") {
			return nil
		}
		return err
	})
}

func (g gotdEngine) Download(ctx context.Context, apiID int, cred *credentials, ref objectRef, messageID int) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		err := g.run(ctx, apiID, cred, func(ctx context.Context, client *telegram.Client) error {
			loc, empty, err := documentLocation(ctx, client.API(), ref, messageID)
			if err != nil || empty {
				return err
			}
			_, err = downloader.NewDownloader().Download(client.API(), loc).Stream(ctx, pw)
			return err
		})
		_ = pw.CloseWithError(err)
	}()
	return pr, nil
}

func userLabel(user tg.UserClass) string {
	account, ok := user.(*tg.User)
	if !ok || account == nil {
		return "Telegram account"
	}
	name := strings.TrimSpace(strings.TrimSpace(account.FirstName) + " " + strings.TrimSpace(account.LastName))
	if account.Username != "" {
		if name != "" {
			return name + " (@" + account.Username + ")"
		}
		return "@" + account.Username
	}
	if name == "" {
		return "Telegram account"
	}
	return name
}

func userPremium(user tg.UserClass) bool {
	account, ok := user.(*tg.User)
	return ok && account != nil && account.GetPremium()
}

func inputPeer(peer storedPeer) tg.InputPeerClass {
	switch peer.Kind {
	case "user":
		return &tg.InputPeerUser{UserID: peer.ID, AccessHash: peer.Access}
	case "chat":
		return &tg.InputPeerChat{ChatID: peer.ID}
	case "channel":
		return &tg.InputPeerChannel{ChannelID: peer.ID, AccessHash: peer.Access}
	default:
		return &tg.InputPeerSelf{}
	}
}

func randomID() (int64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff), nil
}

func sentMessageID(updates tg.UpdatesClass, randomID int64) (int, error) {
	switch batch := updates.(type) {
	case *tg.UpdateShortSentMessage:
		return batch.ID, nil
	case *tg.Updates:
		return idFromUpdates(batch.Updates, randomID)
	case *tg.UpdatesCombined:
		return idFromUpdates(batch.Updates, randomID)
	default:
		return 0, errors.New("telegram did not return a message id")
	}
}

func idFromUpdates(updates []tg.UpdateClass, randomID int64) (int, error) {
	for _, update := range updates {
		switch item := update.(type) {
		case *tg.UpdateMessageID:
			if item.RandomID == randomID && item.ID != 0 {
				return item.ID, nil
			}
		case *tg.UpdateNewMessage:
			if id := messageID(item.Message); id != 0 {
				return id, nil
			}
		case *tg.UpdateNewChannelMessage:
			if id := messageID(item.Message); id != 0 {
				return id, nil
			}
		}
	}
	return 0, errors.New("telegram did not return a message id")
}

func messageID(msg tg.MessageClass) int {
	switch item := msg.(type) {
	case *tg.Message:
		return item.ID
	case *tg.MessageService:
		return item.ID
	default:
		return 0
	}
}

func resolvePeer(ctx context.Context, api *tg.Client, target string) (storedPeer, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return storedPeer{Kind: "self", Target: ""}, nil
	}
	if hash, ok := inviteHash(target); ok {
		return peerFromInvite(ctx, api, hash, target)
	}
	if username, ok := usernameOf(target); ok {
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		if err != nil {
			return storedPeer{}, err
		}
		peer, err := peerFromResolved(resolved.Peer, resolved.Users, resolved.Chats, target)
		return peer, err
	}
	id, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return storedPeer{}, errors.New("target should be empty for Saved Messages, or a @username, invite link, or numeric id")
	}
	return peerFromDialogs(ctx, api, id, target)
}

func usernameOf(target string) (string, bool) {
	lower := strings.ToLower(target)
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "https://telegram.me/", "http://telegram.me/", "t.me/"} {
		if strings.HasPrefix(lower, prefix) {
			name := strings.Trim(target[len(prefix):], "/")
			if validUsername(name) && !strings.HasPrefix(name, "+") && !strings.HasPrefix(strings.ToLower(name), "joinchat") {
				return name, true
			}
			return "", false
		}
	}
	name := strings.TrimPrefix(target, "@")
	if validUsername(name) {
		return name, true
	}
	return "", false
}

func inviteHash(target string) (string, bool) {
	value := strings.TrimSpace(target)
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "/+"):
		value = value[strings.LastIndex(value, "/+")+2:]
	case strings.Contains(lower, "joinchat/"):
		value = value[strings.Index(lower, "joinchat/")+len("joinchat/"):]
	case strings.HasPrefix(lower, "tg://join?invite="):
		value = value[len("tg://join?invite="):]
	default:
		return "", false
	}
	value = strings.Trim(value, "/")
	if cut := strings.IndexAny(value, "?#"); cut >= 0 {
		value = value[:cut]
	}
	if value == "" {
		return "", false
	}
	return value, true
}

func peerFromInvite(ctx context.Context, api *tg.Client, hash, target string) (storedPeer, error) {
	checked, err := api.MessagesCheckChatInvite(ctx, hash)
	if err != nil {
		return storedPeer{}, err
	}
	if already, ok := checked.(*tg.ChatInviteAlready); ok {
		return peerFromChat(already.Chat, target)
	}
	joined, err := api.MessagesImportChatInvite(ctx, hash)
	if err != nil {
		return storedPeer{}, err
	}
	ok, isOK := joined.(*tg.MessagesChatInviteJoinResultOk)
	if !isOK {
		return storedPeer{}, errors.New("telegram did not return the joined chat")
	}
	var chats []tg.ChatClass
	switch updates := ok.Updates.(type) {
	case *tg.Updates:
		chats = updates.Chats
	case *tg.UpdatesCombined:
		chats = updates.Chats
	}
	if len(chats) == 0 {
		return storedPeer{}, errors.New("telegram did not return the joined chat")
	}
	return peerFromChat(chats[0], target)
}

func peerFromResolved(peer tg.PeerClass, users []tg.UserClass, chats []tg.ChatClass, target string) (storedPeer, error) {
	switch item := peer.(type) {
	case *tg.PeerUser:
		for _, user := range users {
			if account, ok := user.(*tg.User); ok && account.ID == item.UserID {
				return storedPeer{Kind: "user", ID: account.ID, Access: account.AccessHash, Target: target}, nil
			}
		}
	case *tg.PeerChat:
		return storedPeer{Kind: "chat", ID: item.ChatID, Target: target}, nil
	case *tg.PeerChannel:
		for _, chat := range chats {
			if channel, ok := chat.(*tg.Channel); ok && channel.ID == item.ChannelID {
				return storedPeer{Kind: "channel", ID: channel.ID, Access: channel.AccessHash, Target: target}, nil
			}
		}
	}
	return storedPeer{}, errors.New("telegram did not return that chat")
}

func peerFromChat(chat tg.ChatClass, target string) (storedPeer, error) {
	switch item := chat.(type) {
	case *tg.Channel:
		return storedPeer{Kind: "channel", ID: item.ID, Access: item.AccessHash, Target: target}, nil
	case *tg.Chat:
		return storedPeer{Kind: "chat", ID: item.ID, Target: target}, nil
	default:
		return storedPeer{}, errors.New("telegram did not return that chat")
	}
}

func peerFromDialogs(ctx context.Context, api *tg.Client, id int64, target string) (storedPeer, error) {
	marked := channelID(id)
	var offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
	var offsetID int
	var offsetDate int
	for range 8 {
		batch, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: offsetPeer, OffsetID: offsetID, OffsetDate: offsetDate, Limit: 100,
		})
		if err != nil {
			return storedPeer{}, err
		}
		chats, users, dialogs := dialogLists(batch)
		for _, chat := range chats {
			switch item := chat.(type) {
			case *tg.Channel:
				if item.ID == id || item.ID == marked || id == -item.ID {
					return storedPeer{Kind: "channel", ID: item.ID, Access: item.AccessHash, Target: target}, nil
				}
			case *tg.Chat:
				if item.ID == id || item.ID == -id {
					return storedPeer{Kind: "chat", ID: item.ID, Target: target}, nil
				}
			}
		}
		for _, user := range users {
			if account, ok := user.(*tg.User); ok && (account.ID == id || account.ID == -id) {
				return storedPeer{Kind: "user", ID: account.ID, Access: account.AccessHash, Target: target}, nil
			}
		}
		if len(dialogs) == 0 {
			break
		}
		last, ok := dialogs[len(dialogs)-1].(*tg.Dialog)
		if !ok {
			break
		}
		offsetID = last.GetTopMessage()
		offsetPeer = dialogPeer(last.GetPeer(), users, chats)
		if offsetPeer == nil {
			break
		}
	}
	return storedPeer{}, errors.New("that numeric id is not in this account's chats")
}

func channelID(id int64) int64 {
	raw := strconv.FormatInt(id, 10)
	if !strings.HasPrefix(raw, "-100") || len(raw) <= 4 {
		return 0
	}
	n, err := strconv.ParseInt(raw[4:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func dialogLists(batch tg.MessagesDialogsClass) ([]tg.ChatClass, []tg.UserClass, []tg.DialogClass) {
	switch item := batch.(type) {
	case *tg.MessagesDialogs:
		return item.Chats, item.Users, item.Dialogs
	case *tg.MessagesDialogsSlice:
		return item.Chats, item.Users, item.Dialogs
	default:
		return nil, nil, nil
	}
}

func dialogPeer(peer tg.PeerClass, users []tg.UserClass, chats []tg.ChatClass) tg.InputPeerClass {
	switch item := peer.(type) {
	case *tg.PeerUser:
		for _, user := range users {
			if account, ok := user.(*tg.User); ok && account.ID == item.UserID {
				return &tg.InputPeerUser{UserID: account.ID, AccessHash: account.AccessHash}
			}
		}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: item.ChatID}
	case *tg.PeerChannel:
		for _, chat := range chats {
			if channel, ok := chat.(*tg.Channel); ok && channel.ID == item.ChannelID {
				return &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}
			}
		}
	}
	return nil
}

func documentLocation(ctx context.Context, api *tg.Client, ref objectRef, messageID int) (tg.InputFileLocationClass, bool, error) {
	var messages []tg.MessageClass
	if ref.Kind == "channel" {
		box, err := api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: ref.ID, AccessHash: ref.Access},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
		})
		if err != nil {
			return nil, false, err
		}
		messages = channelMessages(box)
	} else {
		box, err := api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}})
		if err != nil {
			return nil, false, err
		}
		messages = messageBox(box)
	}
	for _, msg := range messages {
		item, ok := msg.(*tg.Message)
		if !ok || item.ID != messageID {
			continue
		}
		media, ok := item.Media.(*tg.MessageMediaDocument)
		if !ok {
			return nil, true, nil
		}
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return nil, false, errors.New("telegram did not return a downloadable file; save the parts from the chat in order and join them")
		}
		return &tg.InputDocumentFileLocation{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference}, false, nil
	}
	return nil, false, errors.New("telegram did not return that message")
}

func messageBox(box tg.MessagesMessagesClass) []tg.MessageClass {
	switch item := box.(type) {
	case *tg.MessagesMessages:
		return item.Messages
	case *tg.MessagesMessagesSlice:
		return item.Messages
	case *tg.MessagesChannelMessages:
		return item.Messages
	default:
		return nil
	}
}

func channelMessages(box tg.MessagesMessagesClass) []tg.MessageClass {
	return messageBox(box)
}
