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
	alarmId := hd.hub.UserData.newAlarm(ctx, &AlarmSpecification{
		ExpiryTime: message.ExpiryTime,
		Name:       message.Name,
		StartedAt:  message.StartedAt,
	})
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

	hd.hub.UserData.registerSubscriber(ctx, hd.info)
	fmt.Println("E2")

	responder(ctx, StatusMessage{200}, false)
}

var HandleDismissAlarms MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseDismissAlarmsMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	hd.hub.UserData.dismissAlarms(ctx, message.Ids)

	responder(ctx, StatusMessage{200}, false)
}

var HandleCancelAlarm MyHandlerFunc = func(hd *MyHandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCancelAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	hd.hub.UserData.cancelAlarm(ctx, message.Id)

	responder(ctx, StatusMessage{200}, false)
}

var HandlePauseAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParsePauseAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	hd.hub.UserData.pauseAlarm(ctx, message.Id)

	responder(ctx, StatusMessage{200}, false)
}

var HandleContinueAlarm MyHandlerFunc = func(hd *HandlerData[UserData], ctx context.Context, req []byte, responder Responder) {
	message, err := ParseContinueAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	hd.hub.UserData.continueAlarm(ctx, message.Id)

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
		lb: Lockbox[UserDataContent]{
			value: UserDataContent{
				subscribers:  make(map[*MyClientInfo]bool),
				alarms:       make(map[float64]*StoredAlarm),
				pausedAlarms: make(map[float64]bool),
			},
		},
	}
}

func SendAlarmsToClient(ctx context.Context, info *MyClientInfo, alarms []*StoredAlarm, ketchup bool) error {
	_, err := SendMessageToClient(ctx, info, Some("NOTIFY_ALARM"), Optional[float64]{}, NotifyAlarmMessage{alarms, ketchup})
	return err
}

func SendCanceledAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) error {
	_, err := SendMessageToClient(ctx, info, Some("ALARM_CANCELED"), Optional[float64]{}, AlarmCanceledMessage{Id: alarmId})
	return err
}

func SendExpiredAlarmToClient(ctx context.Context, info *MyClientInfo, alarmId float64) error {
	_, err := SendMessageToClient(ctx, info, Some("ALARM_EXPIRED"), Optional[float64]{}, AlarmExpiredMessage{Id: alarmId})
	return err
}

func SendAlarmsDismissedToClient(ctx context.Context, info *MyClientInfo, ids []float64) error {
	_, err := SendMessageToClient(ctx, info, Some("ALARMS_DISMISSED"), Optional[float64]{}, AlarmsDismissedMessage{Ids: ids})
	return err
}

func SendAlarmPausedToClient(ctx context.Context, info *MyClientInfo, id float64, pausedAtTime time.Time) error {
	_, err := SendMessageToClient(ctx, info, Some("ALARM_PAUSED"), Optional[float64]{}, AlarmPausedMessage{Id: id, PausedAtTime: pausedAtTime})
	return err
}

func SendAlarmContinuedToClient(ctx context.Context, info *MyClientInfo, id float64, newExpiryTime time.Time) error {
	_, err := SendMessageToClient(ctx, info, Some("ALARM_CONTINUED"), Optional[float64]{}, AlarmContinuedMessage{Id: id, NewExpiryTime: newExpiryTime})
	return err
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

func (ud *UserData) newAlarm(ctx context.Context, spec *AlarmSpecification) float64 {
	return ud.lb.WriteReturning(func(value *UserDataContent) float64 {
		newStoredAlarm := &StoredAlarm{
			Spec:               *spec,
			Id:                 value.alarmId,
			OriginalExpiryTime: spec.ExpiryTime,
			PausedAtTime:       Optional[time.Time]{},
		}
		value.alarms[value.alarmId] = newStoredAlarm
		value.alarmId++
		ud.startAlarm(newStoredAlarm)
		for info := range value.subscribers {
			err := SendAlarmsToClient(ctx, info, []*StoredAlarm{newStoredAlarm}, false)
			if err != nil {
				fmt.Println("Could not notify of new alarm: ", err)
			}
			if ctx.Err() != nil {
				return newStoredAlarm.Id
			}
		}
		return newStoredAlarm.Id
	})
}

func (ud *UserData) registerSubscriber(ctx context.Context, client *MyClientInfo) {
	ud.lb.Write(func(value *UserDataContent) {
		value.subscribers[client] = true
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
		err := SendAlarmsToClient(ctx, client, slice, true)
		if err != nil {
			fmt.Println("Could not send alarms to new subscriber: ", err)
		}
	})
}

func (ud *UserData) unregisterSubscriber(client *MyClientInfo) {
	ud.lb.Write(func(value *UserDataContent) {
		delete(value.subscribers, client)
	})
}

func (ud *UserData) expireAlarm(ctx context.Context, alarmId float64) {
	ud.lb.Write(func(value *UserDataContent) {
		value.expiredAlarms = append(value.expiredAlarms, alarmId)
		for info := range value.subscribers {
			err := SendExpiredAlarmToClient(ctx, info, alarmId)
			if err != nil {
				fmt.Println("Could not send alarm expiration to client: ", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	})
}

/**
 * If ids is empty, dismisses all expired alarms
 */
func (ud *UserData) dismissAlarms(ctx context.Context, ids []float64) {
	ud.lb.Write(func(value *UserDataContent) {
		if len(ids) == 0 {
			ids = value.expiredAlarms
		}
		for info := range value.subscribers {
			err := SendAlarmsDismissedToClient(ctx, info, ids)
			if err != nil {
				fmt.Println("Could not send alarm dismissal to client: ", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
		value.expiredAlarms = nil
	})
}

func (ud *UserData) pauseAlarm(ctx context.Context, pauseId float64) {
	ud.lb.Write(func(udc *UserDataContent) {
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
		for info := range udc.subscribers {
			err := SendAlarmPausedToClient(ctx, info, pauseId, pausedAtTime)
			if err != nil {
				fmt.Println("Could not send alarm pausing to client: ", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	})
}

func (ud *UserData) continueAlarm(ctx context.Context, continueId float64) {
	ud.lb.Write(func(udc *UserDataContent) {
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
		for info := range udc.subscribers {
			err := SendAlarmContinuedToClient(ctx, info, continueId, storedAlarm.Spec.ExpiryTime)
			if err != nil {
				fmt.Println("Could not send alarm continuation to client: ", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	})
}

func (ud *UserData) cancelAlarm(ctx context.Context, cancelId float64) {
	ud.lb.Write(func(udc *UserDataContent) {
		l1 := len(udc.alarms)
		delete(udc.alarms, cancelId)
		if l1 == len(udc.alarms) {
			return
		}
		for info := range udc.subscribers {
			err := SendCanceledAlarmToClient(ctx, info, cancelId)
			if err != nil {
				fmt.Println("Could not send alarm cancelation to client: ", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	})
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	hub := newHub(makeUserData())
	hub.lb.Write(func(value *HubContent[UserData]) {
		value.ThreadTimeout = time.Second * 5
		value.On.Unregistered = func(e UnregisterItem[UserData]) {
			hub.UserData.unregisterSubscriber(e.info)
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

	<-ctx.Done()
	hub.Close()
	log.Println("shutting down, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
