package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log"
	"maps"
	"math"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

type UserDataContent struct {
	subscribers   map[*MyClientInfo]bool
	expiredAlarms []float64
	pausedAlarms  map[float64]bool
	alarmId       float64
	alarms        map[float64]*StoredAlarm
}

type UserData struct {
	lb Lockbox[UserDataContent]
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
	ExpiryTime time.Time `json:"expiryTime"`
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"startedAt"`
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
}

var HandleCreateAlarm = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCreateAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	fmt.Printf("Set alarm for %v", message.ExpiryTime)
	alarmId, err := hd.hub.UserData.newAlarm(ctx, &AlarmSpecification{
		ExpiryTime: message.ExpiryTime,
		Name:       message.Name,
		StartedAt:  message.StartedAt,
	})
	if err != nil {
		responder(ctx, err, true)
		return
	}
	responder(ctx, AlarmCreatedMessage{alarmId}, false)
}

var HandleSubscribe = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	fmt.Println("Subscribe", string(req))
	message, err := ParseSubscribeMessage(req)
	if err != nil {
		fmt.Println("E1")
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	_ = message

	err = hd.hub.UserData.registerSubscriber(ctx, hd.info)
	if err != nil {
		responder(ctx, err, true)
		return
	}
	fmt.Println("E2")

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

	responder(ctx, StatusMessage{200}, false)
}

var HandleCancelAlarm MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCancelAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	err = hd.hub.UserData.cancelAlarm(ctx, message.Id)
	if err != nil {
		responder(ctx, err, true)
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandlePauseAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParsePauseAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	err = hd.hub.UserData.pauseAlarm(ctx, message.Id)
	if err != nil {
		responder(ctx, err, true)
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandleContinueAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParseContinueAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	err = hd.hub.UserData.continueAlarm(ctx, message.Id)
	if err != nil {
		responder(ctx, err, true)
	}

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
	timeout = time.Duration(math.MaxInt64)
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
		log.Printf("Error in request-thread: %v\n", ret)
	}
	return ret
}

func SendAlarmsToClient(ctx context.Context, info *MyClientInfo, alarms []*StoredAlarm, ketchup bool) error {
	return SendAndWaitFor200(ctx, info, "NOTIFY_ALARM", NotifyAlarmMessage{alarms, ketchup})
}

func SendCanceledAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) error {
	return SendAndWaitFor200(ctx, info, "ALARM_CANCELED", AlarmCanceledMessage{Id: alarmId})
}

func SendExpiredAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) error {
	return SendAndWaitFor200(ctx, info, "ALARM_EXPIRED", AlarmExpiredMessage{Id: alarmId})
}

func SendAlarmsDismissedToClient(ctx context.Context, info *MyClientInfo, ids []float64) error {
	return SendAndWaitFor200(ctx, info, "ALARM_DISMISSED", AlarmsDismissedMessage{Ids: ids})
}

func SendAlarmPausedToClient(ctx context.Context, info *MyClientInfo, id float64, pausedAtTime time.Time) error {
	return SendAndWaitFor200(ctx, info, "ALARM_PAUSED", AlarmPausedMessage{Id: id, PausedAtTime: pausedAtTime})
}

func SendAlarmContinuedToClient(ctx context.Context, info *MyClientInfo, id float64, newExpiryTime time.Time) error {
	return SendAndWaitFor200(ctx, info, "ALARM_CONTINUED", AlarmContinuedMessage{Id: id, NewExpiryTime: newExpiryTime})
}

func (ud *UserData) startAlarm(alarm *StoredAlarm) {
	ctx2, cancel2 := context.WithDeadline(context.Background(), alarm.Spec.ExpiryTime)
	if alarm.cancel != nil {
		alarm.cancel()
	}
	alarm.cancel = cancel2
	id := alarm.Id
	go func() {
		<-ctx2.Done()
		if ctx2.Err() == context.DeadlineExceeded {
			// alarm must have expired, since only the new deadline triggered
			ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
			ud.expireAlarm(ctx3, id)
			cancel3()
		}
		cancel2()
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
		ud.startAlarm(newStoredAlarm)
		wg := sync.WaitGroup{}
		for info := range value.subscribers {
			wg.Go(func() {
				ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
				err := SendAlarmsToClient(ctxPerSub, info, []*StoredAlarm{newStoredAlarm}, false)
				cancelPerSub()
				if err != nil {
					fmt.Println("Could not notify of new alarm: ", err)
				}
			})
		}
		wg.Wait()
		return newStoredAlarm.Id
	})
}

func (ud *UserData) registerSubscriber(ctx context.Context, client *MyClientInfo) error {
	err := ud.lb.Write(ctx, func(value *UserDataContent) {
		value.subscribers[client] = true
	})
	if err != nil {
		log.Printf("Could not register subscriber: %v\n", err)
		return err
	}
	err, err2 := ud.lb.ReadReturning(ctx, func(value *UserDataContent) error {
		slice := slices.Collect(maps.Values(value.alarms))
		slices.SortFunc(slice, func(a *StoredAlarm, b *StoredAlarm) int {
			if a.Spec.StartedAt.Before(b.Spec.StartedAt) {
				return -1
			}
			if a.Spec.StartedAt.Equal(b.Spec.StartedAt) {
				return 0
			}
			return 1
		})
		err = SendAlarmsToClient(ctx, client, slice, true)
		if err != nil {
			log.Println("Could not send alarms to new subscriber: ", err)
			return err
		}
		return nil
	})
	if err2 != nil {
		return err2
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
	return ud.lb.Write(ctx, func(value *UserDataContent) {
		alarm, exists := value.alarms[alarmId]
		if !exists {
			return
		}
		alarm.IsExpired = true
		value.expiredAlarms = append(value.expiredAlarms, alarmId)
		wg := sync.WaitGroup{}
		for info := range value.subscribers {
			wg.Go(func() {
				ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
				err := SendExpiredAlarmToClient(ctxPerSub, info, alarmId)
				cancelPerSub()
				if err != nil {
					fmt.Println("Could not send alarm expiration to client: ", err)
				}
			})
		}
		wg.Wait()
	})
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
		}
		wg := sync.WaitGroup{}
		for info := range value.subscribers {
			wg.Go(func() {
				ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
				err := SendAlarmsDismissedToClient(ctxPerSub, info, ids)
				cancelPerSub()
				if err != nil {
					fmt.Println("Could not send alarm dismissal to client: ", err)
				}
			})
		}
		wg.Wait()
		value.expiredAlarms = nil
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
				ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
				err := SendAlarmPausedToClient(ctxPerSub, info, pauseId, pausedAtTime)
				cancelPerSub()
				if err != nil {
					fmt.Println("Could not send alarm pausing to client: ", err)
				}
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
		storedAlarm.Spec.ExpiryTime = storedAlarm.Spec.ExpiryTime.Add(pausedDuration)
		ud.startAlarm(storedAlarm)
		wg := sync.WaitGroup{}
		for info := range udc.subscribers {
			wg.Go(
				func() {
					ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
					err := SendAlarmContinuedToClient(ctxPerSub, info, continueId, storedAlarm.Spec.ExpiryTime)
					cancelPerSub()
					if err != nil {
						fmt.Println("Could not send alarm continuation to client: ", err)
					}
				},
			)
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
				ctxPerSub, cancelPerSub := DerivedFromTwoContexts(ctx, info.ctx)
				err := SendCanceledAlarmToClient(ctxPerSub, info, cancelId)
				cancelPerSub()
				if err != nil {
					fmt.Println("Could not send alarm cancelation to client: ", err)
				}
			})
		}
		wg.Wait()
	})
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	hub := newHub(makeUserData())
	ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
	err := hub.lb.Write(ctx2, func(value *HubContent[UserData]) {
		value.ThreadTimeout = time.Second * 5
		value.On.Unregistered = func(e UnregisterItem[UserData]) {
			fmt.Print("On unregister")
			unregCtx, unregCancel := context.WithTimeout(context.Background(), time.Second*60)
			err := hub.UserData.unregisterSubscriber(unregCtx, e.info)
			if err != nil {
				log.Printf("Could not unregister subscriber: %v\n", err)
			}
			unregCancel()
		}
	})
	if err != nil {
		fmt.Printf("Could not initialize hub: %v\n", err)
		return
	}
	err = registerHandlers(ctx2, hub)
	if err != nil {
		fmt.Printf("Could not register handlers: %v\n", err)
		return
	}
	cancel2()

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
	log.Println("shutting down, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
