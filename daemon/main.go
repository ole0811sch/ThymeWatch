//go:generate go run github.com/ogen-go/ogen/cmd/ogen --target generated/api -package api --clean ../schema.yaml
package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"encoding/json/jsontext"
	"encoding/json/v2"

	"github.com/coder/websocket"
)

type Hub struct {
	clients            map[*ClientInfo]bool
	subscribers        map[*ClientInfo]bool
	expiredAlarms      []float64
	newAlarm           chan AlarmSpecification
	newAlarmRes        chan float64
	register           chan *ClientInfo
	unregister         chan UnregisterItem
	registerSubscriber chan *ClientInfo
	alarmId            float64
	cancelAlarm        chan float64
	expiredAlarm       chan float64
	dismissAlarms      chan []float64
}

type UnregisterItem struct {
	info   *ClientInfo
	status websocket.StatusCode
	reason string
}

type OutChanItem struct {
	body   any
	header MessageHeader
}

type ClientInfo struct {
	idChan         <-chan float64
	outChan        chan OutChanItem
	conn           *websocket.Conn
	isUnregistered *atomic.Bool
	cancel         context.CancelFunc
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

var alarms map[float64]StoredAlarm = make(map[float64]StoredAlarm)

func newHub() *Hub {
	return &Hub{
		clients:            make(map[*ClientInfo]bool),
		subscribers:        make(map[*ClientInfo]bool),
		newAlarm:           make(chan AlarmSpecification),
		newAlarmRes:        make(chan float64),
		register:           make(chan *ClientInfo),
		unregister:         make(chan UnregisterItem),
		registerSubscriber: make(chan *ClientInfo),
		cancelAlarm:        make(chan float64),
		expiredAlarm:       make(chan float64),
		dismissAlarms:      make(chan []float64),
	}
}

func Some[T any](value T) Optional[T] {
	return Optional[T]{value: value, present: true}
}

func SendAlarmsToClient(ctx context.Context, info *ClientInfo, alarms []StoredAlarm, ketchup bool) {
	SendMessageToClient(ctx, info, Some("NOTIFY_ALARM"), Optional[float64]{}, NotifyAlarmMessage{alarms, ketchup})
}

// returns id, ctx.Err()
func SendMessageToClient(ctx context.Context, info *ClientInfo, tag Optional[string], inReplyTo Optional[float64], body any) (float64, error) {
	var id float64
	select {
	case <-ctx.Done():
		return id, ctx.Err()
	case id = <-info.idChan:
	}
	header := MessageHeader{
		Id:        id,
		InReplyTo: inReplyTo,
		Tag:       tag,
	}
	select {
	case <-ctx.Done():
		return id, ctx.Err()
	case info.outChan <- OutChanItem{header: header, body: body}:
	}
	return id, nil
}

func SendCanceledAlarmToClient(ctx context.Context, info *ClientInfo, alarmId float64) {
	SendMessageToClient(ctx, info, Some("ALARM_CANCELED"), Optional[float64]{}, AlarmCanceledMessage{Id: alarmId})
}

func SendExpiredAlarmToClient(ctx context.Context, info *ClientInfo, alarmId float64) {
	SendMessageToClient(ctx, info, Some("ALARM_EXPIRED"), Optional[float64]{}, AlarmExpiredMessage{Id: alarmId})
}

func SendAlarmsDismissedToClient(ctx context.Context, info *ClientInfo, ids []float64) {
	SendMessageToClient(ctx, info, Some("ALARMS_DISMISSED"), Optional[float64]{}, AlarmsDismissedMessage{Ids: ids})
}

func (h *Hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Hub: close")
			for c := range h.clients {
				c.conn.CloseNow()
			}
			return
		case c := <-h.register:
			fmt.Println("Hub: register")
			h.clients[c] = true
		case info := <-h.registerSubscriber:
			fmt.Println("Hub: register subscriber")
			h.subscribers[info] = true
			slice := slices.Collect(maps.Values(alarms))
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
		case item := <-h.unregister:
			fmt.Println("Hub: unregister")
			delete(h.clients, item.info)
			delete(h.subscribers, item.info)
			item.info.conn.Close(item.status, item.reason)
		case newAlarm := <-h.newAlarm:
			ctx2, cancel2 := context.WithDeadline(ctx, newAlarm.ExpiryTime)
			newStoredAlarm := StoredAlarm{
				Spec:   newAlarm,
				Id:     h.alarmId,
				cancel: cancel2,
			}
			alarms[h.alarmId] = newStoredAlarm
			h.newAlarmRes <- h.alarmId
			h.alarmId++
			fmt.Println(h.subscribers)
			for info := range h.subscribers {
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
					h.expiredAlarm <- newStoredAlarm.Id
				}
			}()
		case alarmId := <-h.expiredAlarm:
			h.expiredAlarms = append(h.expiredAlarms, alarmId)
			for info := range h.subscribers {
				SendExpiredAlarmToClient(ctx, info, alarmId)
				if ctx.Err() != nil {
					return
				}
			}
		case ids := <-h.dismissAlarms:
			if len(ids) == 0 {
				ids = h.expiredAlarms
			}
			for info := range h.subscribers {
				SendAlarmsDismissedToClient(ctx, info, ids)
				if ctx.Err() != nil {
					return
				}
			}
			h.expiredAlarms = nil
		case canceledId := <-h.cancelAlarm:
			l1 := len(alarms)
			delete(alarms, canceledId)
			if l1 == len(alarms) {
				continue
			}
			for info := range h.subscribers {
				SendCanceledAlarmToClient(ctx, info, canceledId)
				if ctx.Err() != nil {
					return
				}
			}
		}
	}
}

func Unregister(ctx context.Context, info *ClientInfo, status websocket.StatusCode, reason string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case hub.unregister <- UnregisterItem{info, status, reason}:
		return nil
	}
}

var hub *Hub = newHub()

// Exactly one of Tag and InReplyTo should be zero
type MessageHeader struct {
	InReplyTo Optional[float64] `json:"inReplyTo"`
	Tag       Optional[string]  `json:"tag"`
	Id        float64           `json:"id"`
}

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

type SubscribeMessage struct {
}

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

type HandlerData struct {
	info     *ClientInfo
	channels HandlerChannels
}

type HandlerChannels struct {
	// (data, id), received by us
	in chan Pair[[]byte, float64]
	// (data, replyingTo, isError), send by us
	out chan Triple[any, float64, bool]
}

type Responder func(context.Context, any, bool) (Receiver, error)
type Receiver func(context.Context) ([]byte, Responder, error)

func MakeSender(channels *HandlerChannels, replyingTo float64) Responder {
	used := false
	return func(ctx context.Context, data any, isErr bool) (Receiver, error) {
		if used {
			return nil, fmt.Errorf("Sender was used already")
		}
		used = true
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case channels.out <- MakeTriple(data, replyingTo, isErr):
			if isErr {
				return nil, nil
			}
			return MakeReceiver(channels), nil
		}
	}
}

func MakeReceiver(channels *HandlerChannels) Receiver {
	used := false
	return func(ctx context.Context) ([]byte, Responder, error) {
		if used {
			return nil, nil, fmt.Errorf("Receiver was used already")
		}
		used = true
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case event := <-channels.in:
			return event.First, MakeSender(channels, event.Second), nil
		}
	}
}

type Pair[T, U any] struct {
	First  T
	Second U
}

type Triple[T, U, V any] struct {
	First  T
	Second U
	Third  V
}

type Optional[T any] struct {
	present bool
	value   T
}

func (opt *Optional[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if opt.present {
		return json.MarshalEncode(enc, opt.value)
	} else {
		enc.WriteToken(jsontext.Null)
		return nil
	}
}

func (opt *Optional[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	kind := dec.PeekKind()
	switch kind {
	case jsontext.KindNull:
		opt.present = false
		if _, err := dec.ReadToken(); err != nil {
			return err
		}
	default:
		var v T
		if err := json.UnmarshalDecode(dec, &v); err != nil {
			return err
		}
		fmt.Println("Un1", v)
		opt.present = true
		opt.value = v
	}
	return nil
}

func MakePair[T, U any](first T, second U) Pair[T, U] {
	return Pair[T, U]{first, second}
}

func MakeTriple[T, U, V any](first T, second U, third V) Triple[T, U, V] {
	return Triple[T, U, V]{first, second, third}
}

func Todo[T any]() T {
	panic("Not implemented")
}

type HandlerFunc func(*HandlerData, context.Context, []byte, Responder)

var HandleCreateAlarm = func(hd *HandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCreateAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	fmt.Printf("Set alarm for %v", message.ExpiryTime)
	select {
	case <-ctx.Done():
		return
	case hub.newAlarm <- AlarmSpecification{ExpiryTime: message.ExpiryTime, Name: message.Name, StartedAt: message.StartedAt}:
	}
	alarmId := <-hub.newAlarmRes
	responder(ctx, AlarmCreatedMessage{alarmId}, false)
}

var HandleSubscribe = func(hd *HandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseSubscribeMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}
	_ = message

	select {
	case <-ctx.Done():
		return
	case hub.registerSubscriber <- hd.info:
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandleDismissAlarms HandlerFunc = func(hd *HandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseDismissAlarmsMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	select {
	case <-ctx.Done():
		return
	case hub.dismissAlarms <- message.Ids:
	}

	responder(ctx, StatusMessage{200}, false)
}

var HandleCancelAlarm HandlerFunc = func(hd *HandlerData, ctx context.Context, req []byte, responder Responder) {
	message, err := ParseCancelAlarmMessage(req)
	if err != nil {
		responder(ctx, ProtocolErrorFromErr(err), true)
		return
	}

	select {
	case <-ctx.Done():
		return
	case hub.cancelAlarm <- message.Id:
	}

	responder(ctx, StatusMessage{200}, false)
}

var handlers = map[string]HandlerFunc{
	"CREATE_ALARM":   HandleCreateAlarm,
	"SUBSCRIBE":      HandleSubscribe,
	"CANCEL_ALARM":   HandleCancelAlarm,
	"DISMISS_ALARMS": HandleDismissAlarms,
}

func handleOutgoingMessages(ctx context.Context, info *ClientInfo) {
	var err = func() error {
		for {
			fmt.Println("Reading outChan")
			select {
			case <-ctx.Done():
				return nil
			case event := <-info.outChan:
				sHeader, err := json.Marshal(event.header)
				if err != nil {
					return err
				}
				_ = sHeader
				sBody, err := json.Marshal(event.body)
				if err != nil {
					return err
				}
				_ = sBody
				writer, err := info.conn.Writer(ctx, websocket.MessageText)
				if err != nil {
					return err
				}
				defer writer.Close()
				for _, msg := range [][]byte{sHeader, {'\n'}, sBody} {
					_, err = writer.Write(msg)
					if err != nil {
						return err
					}
				}
				err = writer.Close()
				if err != nil {
					return err
				}
			}
		}
	}()
	if err != nil {
		fmt.Println(err)
		info.cancel()
	}
	fmt.Println("Close outgoing messages")
}

func handleThreadMessages(ctx context.Context, info *ClientInfo, handlerData *HandlerData, threads *OpenThreads) {
	var event Triple[any, float64, bool]
	select {
	case <-ctx.Done():
		return
	case event = <-handlerData.channels.out:
	}

	if event.Third {
		protErr, ok := event.First.(websocket.CloseError)
		if ok {
			closeWithError(protErr.Code, protErr.Reason, info)
		} else {
			closeWithError(websocket.StatusInternalError, event.First.(error).Error(), info)
		}
		return
	}

	id, err := SendMessageToClient(ctx, info, Optional[string]{}, Some(event.Second), event.First)
	if err != nil {
		return // ctx is done
	}

	WithLock(&threads.mutex, func() {
		threads.msgIdToHandlerData[id] = handlerData
		threads.handlerDataToMsgId[handlerData] = id
	})
}

func handleIdDistribution(ctx context.Context, idChan chan float64) {
	id := 0.0
	for {
		select {
		case <-ctx.Done():
			return
		case idChan <- id:
			id++
		}
	}
}

func closeWithError(status websocket.StatusCode, reason string, info *ClientInfo) {
	if info.isUnregistered.Swap(true) {
		return
	}
	if len(reason) > 125 {
		reason = reason[:125]
	}
	hub.unregister <- UnregisterItem{info, status, reason}
	info.cancel()
}

type OpenThreads struct {
	msgIdToHandlerData map[float64]*HandlerData
	handlerDataToMsgId map[*HandlerData]float64
	mutex              sync.RWMutex
}

func WithLock(lock sync.Locker, f func()) {
	lock.Lock()
	defer lock.Unlock()
	f()
}

func WithLockReturning[T any](lock sync.Locker, f func() T) T {
	lock.Lock()
	defer lock.Unlock()
	return f()
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:5173"},
	})
	if err != nil {
		log.Printf("accept failed: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())

	idChan := make(chan float64)
	outChan := make(chan OutChanItem)
	isUnregistered := atomic.Bool{}
	info := &ClientInfo{idChan: idChan, outChan: outChan, conn: conn, isUnregistered: &isUnregistered, cancel: cancel}
	go handleIdDistribution(ctx, idChan)
	go handleOutgoingMessages(ctx, info)

	hub.register <- info
	closeWithError := func(status websocket.StatusCode, reason string) {
		closeWithError(status, reason, info)
	}
	defer closeWithError(websocket.StatusNormalClosure, "")
	// TODO: update
	openThreads := OpenThreads{
		msgIdToHandlerData: make(map[float64]*HandlerData),
		handlerDataToMsgId: make(map[*HandlerData]float64),
	}

	for {
		ctx2, cancel2 := context.WithCancel(ctx)
		_, msg, err := conn.Read(ctx2)
		cancel2()
		if err != nil {
			fmt.Printf("Handler: err read, %v\n", err)
			return // context cancelled or connection closed
		}

		endHeader := slices.Index(msg, '\n')
		if endHeader == -1 {
			closeWithError(websocket.StatusProtocolError, "Message does not start with header")
			return
		}
		header := MessageHeader{}
		fmt.Println(string(msg[:endHeader]))
		err = json.Unmarshal(msg[:endHeader], &header)
		if err != nil {
			fmt.Println(err.Error())
			closeWithError(websocket.StatusProtocolError, "Invalid header")
		}
		fmt.Println(header)
		if header.Tag.present {
			handlerFn := handlers[header.Tag.value]
			if handlerFn == nil {
				closeWithError(websocket.StatusProtocolError, "Invalid tag")
				return
			}
			go func() {
				handlerData := &HandlerData{
					info: info,
					channels: HandlerChannels{
						in:  make(chan Pair[[]byte, float64]),
						out: make(chan Triple[any, float64, bool]),
					},
				}
				go handleThreadMessages(ctx, info, handlerData, &openThreads)
				handlerFn(handlerData, ctx, msg[endHeader+1:], MakeSender(&handlerData.channels, header.Id))
				WithLock(&openThreads.mutex, func() {
					id := openThreads.handlerDataToMsgId[handlerData]
					l1 := len(openThreads.handlerDataToMsgId)
					delete(openThreads.handlerDataToMsgId, handlerData)
					if len(openThreads.handlerDataToMsgId) < l1 {
						delete(openThreads.msgIdToHandlerData, id)
					}
				})
			}()
		} else {
			data := WithLockReturning(&openThreads.mutex, func() *HandlerData {
				data := openThreads.msgIdToHandlerData[header.InReplyTo.value]
				delete(openThreads.msgIdToHandlerData, header.InReplyTo.value)
				delete(openThreads.handlerDataToMsgId, data)
				return data
			})
			if data == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case data.channels.in <- MakePair(msg[endHeader+1:], header.Id):
			}
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(handleWS),
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	go hub.run(ctx)

	<-ctx.Done()
	log.Println("shutting down, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
