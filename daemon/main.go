package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/samber/lo"
	"org.thymewatch.daemon/ctxlog"
)

var ALARM_PATH = []string{"assets", "Alarm01.mp3"}

type UserDataContent struct {
	subscribers map[*MyClientInfo]bool
	// alarms that have expired but aren't dismissed
	expiredAlarms []float64
	pausedAlarms  map[float64]bool
	alarmId       float64
	alarms        map[float64]*StoredAlarm
}

type UserData struct {
	sharedAlarmSound SharedAlarmSound
	lb               Lockbox[UserDataContent]
}

type (
	MyHub         = Hub[UserData]
	MyHandlerData = HandlerData[UserData]
	MyHandlerFunc = HandlerFunc[UserData]
	MyClientInfo  = ClientInfo[UserData]
)

type StatusMessage struct {
	Status int `json:"status"`
}

func ParseStatusMessage(bytes []byte) (StatusMessage, error) {
	var message StatusMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type AlarmCreatedMessage struct {
	Id float64 `json:"id"`
}

func ProtocolErrorFromErr(err error) websocket.CloseError {
	return websocket.CloseError{
		Code:   websocket.StatusProtocolError,
		Reason: err.Error(),
	}
}

var missingFieldError = websocket.CloseError{
	Code:   websocket.StatusProtocolError,
	Reason: "A field is missing or null",
}

var mistypedFieldError = websocket.CloseError{
	Code:   websocket.StatusProtocolError,
	Reason: "A field has the wrong type",
}

var invalidValueError = websocket.CloseError{
	Code:   websocket.StatusProtocolError,
	Reason: "A field has an invalid value",
}

type AlarmSpecification struct {
	// When paused, this is the expiry time when the alarm was paused
	ExpiryTime time.Time        `json:"expiryTime"`
	Name       Optional[string] `json:"name"`
	StartedAt  time.Time        `json:"startedAt"`
}

type CreateAlarmMessage AlarmSpecification

func ParseCreateAlarmMessage(bytes []byte) (CreateAlarmMessage, error) {
	var message CreateAlarmMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type SubscribeMessage struct{}

func ParseSubscribeMessage(bytes []byte) (SubscribeMessage, error) {
	var message SubscribeMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type CancelAlarmMessage struct {
	Id float64 `json:"id"`
}

func ParseCancelAlarmMessage(bytes []byte) (CancelAlarmMessage, error) {
	var message CancelAlarmMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type DismissAlarmsMessage struct {
	Ids []float64 `json:"ids"`
}

func ParseDismissAlarmsMessage(bytes []byte) (DismissAlarmsMessage, error) {
	var message DismissAlarmsMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type PauseAlarmMessage struct {
	Id float64 `json:"id"`
}

func ParsePauseAlarmMessage(bytes []byte) (PauseAlarmMessage, error) {
	var message PauseAlarmMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type ContinueAlarmMessage struct {
	Id float64 `json:"id"`
}

func ParseContinueAlarmMessage(bytes []byte) (ContinueAlarmMessage, error) {
	var message ContinueAlarmMessage
	err := json.Unmarshal(bytes, &message)
	return message, err
}

type NotifyAlarmMessage struct {
	Alarms  []*StoredAlarm `json:"alarms"`
	Ketchup bool           `json:"ketchup"`
}

type AlarmCanceledMessage struct {
	Id float64 `json:"id"`
}

type AlarmExpiredMessage struct {
	Id float64 `json:"id"`
}

type AlarmsDismissedMessage struct {
	Ids []float64 `json:"ids"`
}

type AlarmPausedMessage struct {
	Id           float64   `json:"id"`
	PausedAtTime time.Time `json:"pausedAtTime"`
}

type AlarmContinuedMessage struct {
	Id            float64   `json:"id"`
	NewExpiryTime time.Time `json:"newExpiryTime"`
}

type StoredAlarm struct {
	Spec               AlarmSpecification  `json:"spec"`
	Id                 float64             `json:"id"`
	OriginalExpiryTime time.Time           `json:"originalExpiryTime"`
	PausedAtTime       Optional[time.Time] `json:"pausedAtTime"`
	IsExpired          bool                `json:"isExpired"`
	IsDismissed        bool                `json:"isDismissed"`
	cancel             context.CancelFunc
	// is nil until there is a notification
	closeNotification func()
}

func (sa *StoredAlarm) ClonePtr() *StoredAlarm {
	cloned := new(StoredAlarm)
	*cloned = *sa
	return cloned
}

func (udc *UserDataContent) CopySubscriberList() []*MyClientInfo {
	return slices.Collect(maps.Keys(udc.subscribers))
}

var HandleCreateAlarm = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCreateAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	alarmId, err := hd.hub.UserData.newAlarm(ctx, &AlarmSpecification{
		ExpiryTime: message.ExpiryTime,
		Name:       message.Name,
		StartedAt:  message.StartedAt,
	})
	if err != nil {
		responder(ctx, err, true)
		return
	}
	ctx = ctxlog.WithAlarmId(ctx, alarmId)
	ctxlog.Printf(ctx, "Set alarm for %v", message.ExpiryTime)
	responder(ctx, AlarmCreatedMessage{alarmId}, false)
}

var HandleSubscribe = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	_, err := ParseSubscribeMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	err = hd.hub.UserData.registerSubscriber(ctx, hd.info)
	if err != nil {
		responder(ctx, err, true)
		return
	}
	ctxlog.Printf(ctx, "Client subscribed")

	responder(ctx, StatusMessage{200}, false)
}

var HandleDismissAlarms MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseDismissAlarmsMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	err = hd.hub.UserData.dismissAlarms(ctx, message.Ids)
	if err != nil {
		responder(ctx, err, true)
		return
	}
	ctxlog.Printf(ctx, "Dismissed alarms")

	responder(ctx, StatusMessage{200}, false)
}

var HandleCancelAlarm MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCancelAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	ctx = ctxlog.WithAlarmId(ctx, message.Id)

	err = hd.hub.UserData.cancelAlarm(ctx, message.Id)
	if err != nil {
		responder(ctx, err, true)
	}
	ctxlog.Printf(ctx, "Canceled alarm")

	responder(ctx, StatusMessage{200}, false)
}

var HandlePauseAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParsePauseAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	ctx = ctxlog.WithAlarmId(ctx, message.Id)

	err = hd.hub.UserData.pauseAlarm(ctx, message.Id)
	if err != nil {
		responder(ctx, err, true)
	}
	ctxlog.Printf(ctx, "Paused alarm")

	responder(ctx, StatusMessage{200}, false)
}

var HandleContinueAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParseContinueAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	ctx = ctxlog.WithAlarmId(ctx, message.Id)

	err = hd.hub.UserData.continueAlarm(ctx, message.Id)
	if err != nil {
		responder(ctx, err, true)
	}
	ctxlog.Printf(ctx, "Continued alarm")

	responder(ctx, StatusMessage{200}, false)
}

func registerHandlers(ctx context.Context, hub *MyHub) error {
	hub.RegisterHandler(ctx, "CREATE_ALARM", HandleCreateAlarm)
	hub.RegisterHandler(ctx, "SUBSCRIBE", HandleSubscribe)
	hub.RegisterHandler(ctx, "CANCEL_ALARM", HandleCancelAlarm)
	hub.RegisterHandler(ctx, "DISMISS_ALARMS", HandleDismissAlarms)
	hub.RegisterHandler(ctx, "PAUSE_ALARM", HandlePauseAlarm)
	hub.RegisterHandler(ctx, "CONTINUE_ALARM", HandleContinueAlarm)
	return ctx.Err()
}

func makeUserData() UserData {
	return UserData{
		sharedAlarmSound: MakeShareAlarmSound(ALARM_PATH),
		lb: MakeLockbox(
			UserDataContent{
				subscribers:  make(map[*MyClientInfo]bool),
				alarms:       make(map[float64]*StoredAlarm),
				pausedAlarms: make(map[float64]bool),
			},
		),
	}
}

func SendAndWaitFor200(ctx context.Context, info *MyClientInfo, tag string, body any) error {
	timeout, err := info.hub.ThreadTimeout(ctx)
	if err != nil {
		return err
	}
	_, ret := info.StartRequestThreadCloseOnError(ctx, timeout, tag,
		body,
		func(hd *HandlerData[UserData], ctx2 context.Context, receiver Receiver) (Void, error) {
			bytes, _, err := receiver(ctx2)
			if err != nil {
				return VVoid, err
			}
			msg, err := ParseStatusMessage(bytes)
			if err != nil {
				return VVoid, err
			}
			if msg.Status != 200 {
				return VVoid, fmt.Errorf("Client send status %v", msg.Status)
			}
			return VVoid, nil
		})
	if ret != nil {
		ctxlog.Printf(ctx, "Error in request-thread: %v\n", ret)
	}
	return ret
}

func SendAlarmsToClient(ctx context.Context, info *MyClientInfo, alarms []*StoredAlarm, ketchup bool) error {
	return SendAndWaitFor200(ctx, info, "NOTIFY_ALARM", NotifyAlarmMessage{alarms, ketchup})
}

func SendCanceledAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) error {
	return SendAndWaitFor200(ctxlog.WithAlarmId(ctx, alarmId), info, "ALARM_CANCELED", AlarmCanceledMessage{Id: alarmId})
}

func SendExpiredAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) error {
	return SendAndWaitFor200(ctxlog.WithAlarmId(ctx, alarmId), info, "ALARM_EXPIRED", AlarmExpiredMessage{Id: alarmId})
}

func SendAlarmsDismissedToClient(ctx context.Context, info *MyClientInfo, ids []float64) error {
	return SendAndWaitFor200(ctx, info, "ALARMS_DISMISSED", AlarmsDismissedMessage{Ids: ids})
}

func SendAlarmPausedToClient(ctx context.Context, info *MyClientInfo, id float64, pausedAtTime time.Time) error {
	return SendAndWaitFor200(ctxlog.WithAlarmId(ctx, id), info, "ALARM_PAUSED", AlarmPausedMessage{Id: id, PausedAtTime: pausedAtTime})
}

func SendAlarmContinuedToClient(ctx context.Context, info *MyClientInfo, id float64, newExpiryTime time.Time) error {
	return SendAndWaitFor200(ctxlog.WithAlarmId(ctx, id), info, "ALARM_CONTINUED", AlarmContinuedMessage{Id: id, NewExpiryTime: newExpiryTime})
}

func (ud *UserData) startAlarm(alarm *StoredAlarm) {
	ctx := ctxlog.WithAlarmId(context.Background(), alarm.Id)
	ctx2, cancel2 := context.WithDeadline(ctx, alarm.Spec.ExpiryTime)
	if alarm.cancel != nil {
		alarm.cancel()
	}
	alarm.cancel = cancel2
	id := alarm.Id
	go func() {
		<-ctx2.Done()
		defer cancel2()
		if ctx2.Err() != context.DeadlineExceeded {
			return
		}
		// alarm must have expired, since only the new deadline triggered
		ctxWithId := ctxlog.WithAlarmId(context.Background(), id)
		ctx3, cancel3 := context.WithTimeout(ctxWithId, 5*time.Second)
		err := ud.expireAlarm(ctx3, id)
		cancel3()
		if err == nil {
			ctxlog.Printf(ctxWithId, "Alarm expired")
			return
		}
		ctxlog.Printf(ctxWithId, "Error while handling expired alarm: %v", err)
		go func() {
			ctxNotif, cancelNotif := context.WithTimeout(ctxWithId, 5*time.Second)
			_, errNotif := Notify(ctxNotif, "Error while handling expired alarm",
				fmt.Sprintf("Reason: %v", err),
				func(id uint32) { cancelNotif() }, nil, NotificationOptions{
					expireTimeout: 0, urgency: 2,
				})
			if errNotif == nil {
				return
			}
			ctxlog.Printf(ctxWithId, "Could not notify about error while handling expired alarm: %v\n", errNotif)
		}()
	}()
}

func (ud *UserData) stopAlarm(alarm *StoredAlarm) {
	if alarm.cancel == nil {
		return
	}
	alarm.cancel()
	alarm.cancel = nil
}

func (ud *UserData) newAlarm(ctx context.Context, spec *AlarmSpecification) (float64, error) {
	if maContext == nil {
		go func() {
			err := InitAudio()
			if err == nil {
				return
			}
			ctxlog.Printf(ctx, "Could not initialize audio: %v", err)
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			_, err = Notify(ctx, "Could not initialize audio",
				fmt.Sprintf("Alarms will play no sound\nReason: %v", err),
				func(id uint32) { cancel() }, nil, NotificationOptions{
					expireTimeout: 0,
					urgency:       2,
				})
			if err != nil {
				ctxlog.Printf(ctx, "Could not notify about broken audio: %v", err)
			}
		}()
	}
	return ud.lb.WriteReturning(ctx, func(value *UserDataContent) float64 {
		newStoredAlarm := &StoredAlarm{
			Spec:               *spec,
			Id:                 value.alarmId,
			OriginalExpiryTime: spec.ExpiryTime,
			PausedAtTime:       Optional[time.Time]{},
			IsExpired:          false,
			IsDismissed:        false,
		}
		value.alarms[value.alarmId] = newStoredAlarm
		value.alarmId++
		ctx = ctxlog.WithAlarmId(ctx, newStoredAlarm.Id)
		ud.startAlarm(newStoredAlarm)
		newStoredAlarmClone := newStoredAlarm.ClonePtr()
		wg := sync.WaitGroup{}
		for info := range value.subscribers {
			wg.Go(func() {
				info.ScheduleTask(
					ctx,
					"NotifyNewAlarm",
					func(ctx context.Context, info *ClientInfo[UserData]) error {
						ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
						err := SendAlarmsToClient(ctxPerSub, info, []*StoredAlarm{newStoredAlarmClone}, false)
						cancelPerSub()
						return err
					},
				)
			})
		}
		wg.Wait()
		return newStoredAlarm.Id
	})
}

func (ud *UserData) registerSubscriber(ctx context.Context, client *MyClientInfo) error {
	err, err2 := ud.lb.WriteReturning(ctx, func(value *UserDataContent) error {
		value.subscribers[client] = true
		slice := lo.Map(slices.Collect(maps.Values(value.alarms)),
			Ignoring2ndArg[int]((*StoredAlarm).ClonePtr))
		err := client.ScheduleTask(ctx,
			"AllAlarmsForNewSubscriber",
			func(ctx context.Context, info *ClientInfo[UserData]) error {
				slices.SortFunc(slice, func(a *StoredAlarm, b *StoredAlarm) int {
					if a.Spec.StartedAt.Before(b.Spec.StartedAt) {
						return -1
					}
					if a.Spec.StartedAt.Equal(b.Spec.StartedAt) {
						return 0
					}
					return 1
				})
				return SendAlarmsToClient(ctx, client, slice, true)
			})
		if err != nil {
			delete(value.subscribers, client)
		}
		return err
	})
	if err2 != nil {
		return err
	}
	if err != nil {
		return err
	}
	return nil
}

func (ud *UserData) unregisterSubscriber(ctx context.Context, client *MyClientInfo) error {
	return ud.lb.Write(ctx, func(value *UserDataContent) {
		delete(value.subscribers, client)
	})
}

func (ud *UserData) expireAlarm(ctx context.Context, alarmId float64) error {
	exists, alarmName, err := ud.lb.WriteReturning2(ctx, func(value *UserDataContent) (bool, Optional[string]) {
		alarm, exists := value.alarms[alarmId]
		if !exists {
			return false, Optional[string]{}
		}
		alarm.IsExpired = true
		value.expiredAlarms = append(value.expiredAlarms, alarmId)
		wg := sync.WaitGroup{}
		for info := range value.subscribers {
			wg.Go(func() {
				err := info.ScheduleTask(
					ctx,
					"NotifyAlarmExpiration",
					func(ctx context.Context, info *ClientInfo[UserData]) error {
						ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
						err := SendExpiredAlarmToClient(ctxPerSub, info, alarmId)
						cancelPerSub()
						return err
					},
				)
				LogIfError("Could not schedule alarm expiration task", err)
			})
		}
		wg.Wait()
		return true, alarm.Spec.Name
	})
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	close := make(chan Void, 1)
	go ud.notifyExpiredAlarm(close, alarmId, alarmName)
	return ud.sharedAlarmSound.Play(ctx)
}

func (ud *UserData) notifyExpiredAlarm(close chan Void, alarmId float64, alarmName Optional[string]) {
	ctxWithId := ctxlog.WithAlarmId(context.Background(), alarmId)
	ctx, cancel := context.WithTimeout(ctxWithId, 5*time.Second)
	title := "Alarm expired"
	if alarmName.present {
		title = fmt.Sprintf("Alarm \"%v\" expired", alarmName.value)
	}
	reason, err := Notify(ctx, title, "", func(id uint32) {
		err := ud.lb.Write(ctx, func(value *UserDataContent) {
			alarm, exists := value.alarms[alarmId]
			if !exists {
				return
			}
			alarm.closeNotification = func() {
				select {
				case close <- VVoid:
				default:
				}
			}
		})
		if err != nil {
			ctxlog.Printf(ctx, "Error while handling notifications: %v", err)
		}
		cancel()
	}, close, NotificationOptions{expireTimeout: 0, urgency: 2})
	if err != nil {
		ctxlog.Printf(ctx, "Error while handling notifications: %v", err)
		return
	}
	switch reason {
	case NotificationClosedReasonCall, NotificationClosedReasonDismissed:
		ctxDismiss, cancelDismiss := context.WithTimeout(ctxWithId, 5*time.Second)
		err := ud.dismissAlarms(ctxDismiss, []float64{alarmId})
		cancelDismiss()
		if err != nil {
			ctxlog.Printf(ctx, "Error while dismissing alarm: %v", err)
		}
	case NotificationClosedReasonUndefined:
		ctxlog.Printf(ctxWithId, "Unexpected NotificationClosedReason %v", reason)
	}
}

// If ids is empty, dismisses all expired alarms
func (ud *UserData) dismissAlarms(ctx context.Context, ids []float64) error {
	return ud.lb.Write(ctx, func(value *UserDataContent) {
		if len(ids) == 0 {
			ids = value.expiredAlarms
		}
		for i := 0; i < len(ids); i++ {
			alarm, exists := value.alarms[ids[i]]
			if !exists || alarm.IsDismissed || !alarm.IsExpired {
				ids[i] = ids[len(ids)-1]
				ids = ids[:len(ids)-1]
				continue
			}
			alarm.IsExpired = true
			if alarm.closeNotification != nil {
				alarm.closeNotification()
				alarm.closeNotification = nil
			}
		}
		value.expiredAlarms = nil
		wg := sync.WaitGroup{}
		for info := range value.subscribers {
			wg.Go(func() {
				err := info.ScheduleTask(
					ctx,
					"NotifyAlarmDismissal",
					func(ctx context.Context, info *ClientInfo[UserData]) error {
						ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
						err := SendAlarmsDismissedToClient(ctxPerSub, info, ids)
						cancelPerSub()
						return err
					},
				)
				LogIfError("Could not schedule alarm dismissal task", err)
			})
		}
		wg.Wait()
		err := ud.sharedAlarmSound.Stop(ctx)
		if err != nil {
			ctxlog.Printf(ctx, "Could not stop alarm sound: %v\n", err)
		}
	})
}

func (ud *UserData) pauseAlarm(ctx context.Context, pauseId float64) error {
	return ud.lb.Write(ctx, func(udc *UserDataContent) {
		_, isPausedAlready := udc.pausedAlarms[pauseId]
		if isPausedAlready {
			return
		}
		udc.pausedAlarms[pauseId] = true
		storedAlarm := udc.alarms[pauseId]
		if storedAlarm == nil {
			delete(udc.pausedAlarms, pauseId)
			return
		}
		pausedAtTime := time.Now()
		storedAlarm.PausedAtTime = Some(pausedAtTime)
		ud.stopAlarm(storedAlarm)
		wg := sync.WaitGroup{}
		for info := range udc.subscribers {
			wg.Go(func() {
				err := info.ScheduleTask(
					ctx,
					"NotifyAlarmPausing",
					func(ctx context.Context, info *ClientInfo[UserData]) error {
						ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
						err := SendAlarmPausedToClient(ctxPerSub, info, pauseId, pausedAtTime)
						cancelPerSub()
						return err
					},
				)
				LogIfError("Could not schedule alarm pausing task", err)
			})
		}
		wg.Wait()
	})
}

func (ud *UserData) continueAlarm(ctx context.Context, continueId float64) error {
	return ud.lb.Write(ctx, func(udc *UserDataContent) {
		_, wasPaused := udc.pausedAlarms[continueId]
		if !wasPaused {
			return
		}
		delete(udc.pausedAlarms, continueId)
		storedAlarm := udc.alarms[continueId]
		if storedAlarm == nil {
			return
		}
		if !storedAlarm.PausedAtTime.present {
			// inconsistent view
			return
		}
		pausedDuration := time.Since(storedAlarm.PausedAtTime.value)
		storedAlarm.PausedAtTime.present = false
		newExpiryTime := storedAlarm.Spec.ExpiryTime.Add(pausedDuration)
		storedAlarm.Spec.ExpiryTime = newExpiryTime
		ud.startAlarm(storedAlarm)
		wg := sync.WaitGroup{}
		for info := range udc.subscribers {
			wg.Go(func() {
				err := info.ScheduleTask(
					ctx,
					"NotifyAlarmContinuation",
					func(ctx context.Context, info *ClientInfo[UserData]) error {
						ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
						err := SendAlarmContinuedToClient(ctxPerSub, info, continueId, newExpiryTime)
						cancelPerSub()
						return err
					},
				)
				LogIfError("Could not schedule alarm continuation task", err)
			})
		}
		wg.Wait()
	})
}

func (ud *UserData) cancelAlarm(ctx context.Context, cancelId float64) error {
	return ud.lb.Write(ctx, func(udc *UserDataContent) {
		l1 := len(udc.alarms)
		delete(udc.alarms, cancelId)
		if l1 == len(udc.alarms) {
			return
		}
		wg := sync.WaitGroup{}
		for info := range udc.subscribers {
			wg.Go(func() {
				err := info.ScheduleTask(
					ctx,
					"NotifyAlarmCancelation",
					func(ctx context.Context, info *ClientInfo[UserData]) error {
						ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
						err := SendCanceledAlarmToClient(ctxPerSub, info, cancelId)
						cancelPerSub()
						return err
					},
				)
				LogIfError("Could not schedule alarm cancelation task", err)
			})
		}
		wg.Wait()
	})
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	hub := newHub(makeUserData())
	ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
	err := hub.lb.Write(ctx2, func(value *HubContent[UserData]) {
		value.ThreadTimeout = time.Second * 5
		value.On.Unregistered = func(e UnregisterItem[UserData]) {
			unregCtx, unregCancel := context.WithTimeout(context.Background(), time.Second*60)
			err := hub.UserData.unregisterSubscriber(unregCtx, e.info)
			if err != nil {
				ctxlog.Printf(ctx2, "Could not unregister subscriber: %v\n", err)
			}
			unregCancel()
		}
	})
	if err != nil {
		cancel2()
		ctxlog.Printf(ctx, "Could not initialize hub: %v\n", err)
		return
	}
	err = registerHandlers(ctx2, hub)
	cancel2()
	if err != nil {
		ctxlog.Printf(ctx, "Could not register handlers: %v\n", err)
		return
	}

	srv := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleWS(hub, w, r)
		}),
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	hubCloseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	hub.Close(hubCloseCtx)
	cancel()
	ctxlog.Printf(ctx, "shutting down, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
