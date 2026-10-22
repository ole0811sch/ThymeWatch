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
	newAlarm             chan AlarmSpecification
	newAlarmRes          chan float64
	alarmId              float64
	cancelAlarm          chan float64
	expiredAlarm         chan float64
	dismissAlarms        chan []float64
	registerSubscriber   chan *MyClientInfo
	unregisterSubscriber chan *MyClientInfo
	alarms               map[float64]StoredAlarm
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

type NotifyAlarmMessage struct {
	Alarms  []StoredAlarm `json:"alarms"`
	Ketchup bool          `json:"ketchup"`
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

type StoredAlarm struct {
	Spec   AlarmSpecification `json:"spec"`
	Id     float64            `json:"id"`
	cancel context.CancelFunc
}

var HandleCreateAlarm = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCreateAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	fmt.Printf("Set alarm for %v", message.ExpiryTime)
	select {
	case <-ctx.Done():
		return
	case hd.hub.UserData.newAlarm <- AlarmSpecification{ExpiryTime: message.ExpiryTime, Name: message.Name, StartedAt: message.StartedAt}:
	}
	alarmId := <-hd.hub.UserData.newAlarmRes
	responder(ctx, AlarmCreatedMessage{alarmId}, false)
}

var HandleSubscribe = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseSubscribeMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	_ = message

	select {
	case <-ctx.Done():
		return
	case hd.hub.UserData.registerSubscriber <- hd.info:
	}

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

func registerHandlers(hub *MyHub) {
	hub.RegisterHandler("CREATE_ALARM", HandleCreateAlarm)
	hub.RegisterHandler("SUBSCRIBE", HandleSubscribe)
	hub.RegisterHandler("CANCEL_ALARM", HandleCancelAlarm)
	hub.RegisterHandler("DISMISS_ALARMS", HandleDismissAlarms)
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
		alarms:               make(map[float64]StoredAlarm),
	}
}

func SendAlarmsToClient(ctx context.Context, info *MyClientInfo, alarms []StoredAlarm, ketchup bool) {
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

func (ud *UserData) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case info := <-ud.registerSubscriber:
			fmt.Println("Hub: register subscriber")
			ud.subscribers[info] = true
			slice := slices.Collect(maps.Values(ud.alarms))
			slices.SortFunc(slice, func(a StoredAlarm, b StoredAlarm) int {
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
			ctx2, cancel2 := context.WithDeadline(ctx, newAlarm.ExpiryTime)
			newStoredAlarm := StoredAlarm{
				Spec:   newAlarm,
				Id:     ud.alarmId,
				cancel: cancel2,
			}
			ud.alarms[ud.alarmId] = newStoredAlarm
			ud.newAlarmRes <- ud.alarmId
			ud.alarmId++
			for info := range ud.subscribers {
				SendAlarmsToClient(ctx, info, []StoredAlarm{newStoredAlarm}, false)
				if ctx.Err() != nil {
					return
				}
			}
			go func() {
				<-ctx2.Done()
				if ctx.Err() != nil {
					return
				}
				if ctx2.Err() == context.DeadlineExceeded {
					// alarm must have expired, since only the new deadline triggered
					ud.expiredAlarm <- newStoredAlarm.Id
				}
			}()
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
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	hub := newHub(makeUserData())
	registerHandlers(hub)
	hub.On.Unregistered = func(e UnregisterItem[UserData]) {
		hub.UserData.unregisterSubscriber <- e.info
	}

	srv := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hub.handleWS(w, r)
		}),
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	go hub.run(ctx)
	go hub.UserData.run(ctx)

	<-ctx.Done()
	log.Println("shutting down, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
