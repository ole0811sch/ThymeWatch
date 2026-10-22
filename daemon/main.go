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
	"syscall"
	"time"

	"github.com/coder/websocket"
)

type UserData struct {
	subscribers          map[*MyClientInfo]bool
	expiredAlarms        []float64
	pausedAlarms         map[float64]bool
	newAlarm             chan AlarmSpecification
	newAlarmRes          chan float64
	alarmId              float64
	cancelAlarm          chan float64
	expiredAlarm         chan float64
	pauseAlarm           chan float64
	continueAlarm        chan float64
	dismissAlarms        chan []float64
	registerSubscriber   chan *MyClientInfo
	unregisterSubscriber chan *MyClientInfo
	alarms               map[float64]*StoredAlarm
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
	cancel             context.CancelFunc
}

var HandleCreateAlarm = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCreateAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	fmt.Printf("Set alarm for %v", message.ExpiryTime)
	userData := hd.hub.UserData
	select {
	case <-ctx.Done():
		return
	case userData.newAlarm <- AlarmSpecification{
		ExpiryTime: message.ExpiryTime,
		Name:       message.Name,
		StartedAt:  message.StartedAt,
	}:
	}
	alarmId := <-userData.newAlarmRes
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

	select {
	case <-ctx.Done():
		fmt.Println("E2")
		return
	case hd.hub.UserData.registerSubscriber <- hd.info:
	}
	fmt.Println("E3")

	responder(ctx, StatusMessage{200}, false)
}

var HandleDismissAlarms MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseDismissAlarmsMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	select {
	case <-ctx.Done():
		return
	case hd.hub.UserData.dismissAlarms <- message.Ids:
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandleCancelAlarm MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCancelAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	select {
	case <-ctx.Done():
		return
	case hd.hub.UserData.cancelAlarm <- message.Id:
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandlePauseAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParsePauseAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	select {
	case <-ctx.Done():
		return
	case hd.hub.UserData.pauseAlarm <- message.Id:
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandleContinueAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParseContinueAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	select {
	case <-ctx.Done():
		return
	case hd.hub.UserData.continueAlarm <- message.Id:
	}

	responder(ctx, StatusMessage{200}, false)
}

func registerHandlers(hub *MyHub) {
	hub.RegisterHandler("CREATE_ALARM", HandleCreateAlarm)
	hub.RegisterHandler("SUBSCRIBE", HandleSubscribe)
	hub.RegisterHandler("CANCEL_ALARM", HandleCancelAlarm)
	hub.RegisterHandler("DISMISS_ALARMS", HandleDismissAlarms)
	hub.RegisterHandler("PAUSE_ALARM", HandlePauseAlarm)
	hub.RegisterHandler("CONTINUE_ALARM", HandleContinueAlarm)
}

func makeUserData() UserData {
	return UserData{
		subscribers:          make(map[*MyClientInfo]bool),
		newAlarm:             make(chan AlarmSpecification),
		newAlarmRes:          make(chan float64),
		registerSubscriber:   make(chan *MyClientInfo),
		cancelAlarm:          make(chan float64),
		expiredAlarm:         make(chan float64),
		dismissAlarms:        make(chan []float64),
		unregisterSubscriber: make(chan *MyClientInfo),
		alarms:               make(map[float64]*StoredAlarm),
		pausedAlarms:         make(map[float64]bool),
		pauseAlarm:           make(chan float64),
		continueAlarm:        make(chan float64),
	}
}

func SendAlarmsToClient(ctx context.Context, info *MyClientInfo, alarms []*StoredAlarm, ketchup bool) {
	SendMessageToClient(ctx, info, Some("NOTIFY_ALARM"), Optional[float64]{}, NotifyAlarmMessage{alarms, ketchup})
}

func SendCanceledAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) {
	SendMessageToClient(ctx, info, Some("ALARM_CANCELED"), Optional[float64]{}, AlarmCanceledMessage{Id: alarmId})
}

func SendExpiredAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) {
	SendMessageToClient(ctx, info, Some("ALARM_EXPIRED"), Optional[float64]{}, AlarmExpiredMessage{Id: alarmId})
}

func SendAlarmsDismissedToClient(ctx context.Context, info *MyClientInfo, ids []float64) {
	SendMessageToClient(ctx, info, Some("ALARMS_DISMISSED"), Optional[float64]{}, AlarmsDismissedMessage{Ids: ids})
}

func SendAlarmPausedToClient(ctx context.Context, info *MyClientInfo, id float64, pausedAtTime time.Time) {
	SendMessageToClient(ctx, info, Some("ALARM_PAUSED"), Optional[float64]{},
		AlarmPausedMessage{Id: id, PausedAtTime: pausedAtTime})
}

func SendAlarmContinuedToClient(ctx context.Context, info *MyClientInfo, id float64, newExpiryTime time.Time) {
	SendMessageToClient(ctx, info, Some("ALARM_CONTINUED"), Optional[float64]{},
		AlarmContinuedMessage{Id: id, NewExpiryTime: newExpiryTime})
}

func (ud *UserData) startAlarm(ctx context.Context, alarm *StoredAlarm) {
	ctx2, cancel2 := context.WithDeadline(ctx, alarm.Spec.ExpiryTime)
	if alarm.cancel != nil {
		alarm.cancel()
	}
	alarm.cancel = cancel2
	id := alarm.Id
	go func() {
		<-ctx2.Done()
		if ctx.Err() != nil {
			return
		}
		if ctx2.Err() == context.DeadlineExceeded {
			// alarm must have expired, since only the new deadline triggered
			ud.expiredAlarm <- id
		}
	}()
}

func (ud *UserData) stopAlarm(alarm *StoredAlarm) {
	if alarm.cancel == nil {
		return
	}
	alarm.cancel()
	alarm.cancel = nil
}

func (ud *UserData) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case info := <-ud.registerSubscriber:
			ud.subscribers[info] = true
			slice := slices.Collect(maps.Values(ud.alarms))
			slices.SortFunc(slice, func(a *StoredAlarm, b *StoredAlarm) int {
				if a.Spec.StartedAt.Before(b.Spec.StartedAt) {
					return -1
				}
				if a.Spec.StartedAt.Equal(b.Spec.StartedAt) {
					return 0
				}
				return 1
			})
			SendAlarmsToClient(ctx, info, slice, true)
			if ctx.Err() != nil {
				return
			}
		case newAlarm := <-ud.newAlarm:
			newStoredAlarm := &StoredAlarm{
				Spec:               newAlarm,
				Id:                 ud.alarmId,
				OriginalExpiryTime: newAlarm.ExpiryTime,
				PausedAtTime:       Optional[time.Time]{},
			}
			ud.alarms[ud.alarmId] = newStoredAlarm
			ud.newAlarmRes <- ud.alarmId
			ud.alarmId++
			ud.startAlarm(ctx, newStoredAlarm)
			for info := range ud.subscribers {
				SendAlarmsToClient(ctx, info, []*StoredAlarm{newStoredAlarm}, false)
				if ctx.Err() != nil {
					return
				}
			}
		case alarmId := <-ud.expiredAlarm:
			ud.expiredAlarms = append(ud.expiredAlarms, alarmId)
			for info := range ud.subscribers {
				SendExpiredAlarmToClient(ctx, info, alarmId)
				if ctx.Err() != nil {
					return
				}
			}
		case ids := <-ud.dismissAlarms:
			if len(ids) == 0 {
				ids = ud.expiredAlarms
			}
			for info := range ud.subscribers {
				SendAlarmsDismissedToClient(ctx, info, ids)
				if ctx.Err() != nil {
					return
				}
			}
			ud.expiredAlarms = nil
		case canceledId := <-ud.cancelAlarm:
			l1 := len(ud.alarms)
			delete(ud.alarms, canceledId)
			if l1 == len(ud.alarms) {
				continue
			}
			for info := range ud.subscribers {
				SendCanceledAlarmToClient(ctx, info, canceledId)
				if ctx.Err() != nil {
					return
				}
			}
		case pauseId := <-ud.pauseAlarm:
			_, isPausedAlready := ud.pausedAlarms[pauseId]
			if isPausedAlready {
				continue
			}
			ud.pausedAlarms[pauseId] = true
			storedAlarm := ud.alarms[pauseId]
			if storedAlarm == nil {
				delete(ud.pausedAlarms, pauseId)
				continue
			}
			pausedAtTime := time.Now()
			storedAlarm.PausedAtTime = Some(pausedAtTime)
			ud.stopAlarm(storedAlarm)
			for info := range ud.subscribers {
				SendAlarmPausedToClient(ctx, info, pauseId, pausedAtTime)
				if ctx.Err() != nil {
					return
				}
			}
		case continueId := <-ud.continueAlarm:
			_, wasPaused := ud.pausedAlarms[continueId]
			if !wasPaused {
				continue
			}
			delete(ud.pausedAlarms, continueId)
			storedAlarm := ud.alarms[continueId]
			if storedAlarm == nil {
				continue
			}
			if !storedAlarm.PausedAtTime.present {
				// inconsistent view
				continue
			}
			pausedDuration := time.Since(storedAlarm.PausedAtTime.value)
			storedAlarm.PausedAtTime.present = false
			storedAlarm.Spec.ExpiryTime = storedAlarm.Spec.ExpiryTime.Add(pausedDuration)
			ud.startAlarm(ctx, storedAlarm)
			for info := range ud.subscribers {
				SendAlarmContinuedToClient(ctx, info, continueId, storedAlarm.Spec.ExpiryTime)
				if ctx.Err() != nil {
					return
				}
			}
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	userData := makeUserData()
	hub := newHub(userData)
	hub.lb.Write(func(value *HubContent[UserData]) {
		value.ThreadTimeout = time.Second * 5
		value.On.Unregistered = func(e UnregisterItem[UserData]) {
			userData.unregisterSubscriber <- e.info
		}
	})
	registerHandlers(hub)

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
	go userData.run(ctx)

	<-ctx.Done()
	hub.Close()
	log.Println("shutting down, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
