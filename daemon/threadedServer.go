package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"org.thymewatch.daemon/ctxlog"

	"github.com/coder/websocket"
)

type HubContent[T any] struct {
	handlers map[string]HandlerFunc[T]
	clients  map[*ClientInfo[T]]bool
	// event handlers may still be called shortly after they have removed
	On struct {
		Unregistered func(e UnregisterItem[T])
	}
	ThreadTimeout time.Duration
	// timeout for tasks in ClientInfo.Tasks
	TaskTimeout time.Duration
	isClosed    bool
}

// UserData can be accessed/modified without obtaining a lock (it is assumed to be synchronized internally)
type Hub[T any] struct {
	lb       Lockbox[HubContent[T]]
	UserData T
	closed   chan Void
}

type UnregisterItem[T any] struct {
	info   *ClientInfo[T]
	status websocket.StatusCode
	reason string
}

type OutChanItem struct {
	body   any
	header MessageHeader
}

type ClientTask[T any] struct {
	f    func(ctx context.Context, info *ClientInfo[T]) error
	name string
	// only for ctx.Values()
	ctx context.Context
}

type ClientInfo[T any] struct {
	hub            *Hub[T]
	currId         Lockbox[float64]
	conn           *websocket.Conn
	isUnregistered *atomic.Bool
	cancel         context.CancelFunc
	// context representing the lifetime of the connection
	ctx         context.Context
	OpenThreads Lockbox[OpenThreads[T]]
	// will be executed serially with ctx with timeout hub.TaskTimeout
	Tasks chan ClientTask[T]
	// for logging
	Id int32
}

var (
	lastTraceId  = atomic.Int32{}
	lastClientId = atomic.Int32{}
	_            = func() Void {
		lastTraceId.Store(-1)
		lastClientId.Store(-1)
		return VVoid
	}()
)

func (c *ClientInfo[T]) ObtainId(ctx context.Context) (float64, error) {
	return c.currId.WriteReturning(ctx, func(value *float64) float64 {
		ret := (*value)
		*value++
		return ret
	})
}

func (c *ClientInfo[T]) ScheduleTask(ctx context.Context, name string, task func(ctx context.Context, info *ClientInfo[T]) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.Tasks <- ClientTask[T]{name: name, f: task, ctx: ctx}:
		return nil
	}
}

type HubIsClosedError struct{}

var HubIsClosedErrorInstance = HubIsClosedError{}

func (e *HubIsClosedError) Error() string {
	return "The hub is closed already"
}

func newHub[T any](userData T) *Hub[T] {
	return &Hub[T]{
		lb: MakeLockbox(
			HubContent[T]{
				handlers:      make(map[string]HandlerFunc[T]),
				clients:       make(map[*ClientInfo[T]]bool),
				ThreadTimeout: 5 * time.Second,
				TaskTimeout:   5 * time.Second,
			},
		),
		UserData: userData,
		closed:   make(chan Void, 1),
	}
}

func Some[T any](value T) Optional[T] {
	return Optional[T]{value: value, present: true}
}

func SendMessageToClient[T any](ctx context.Context, info *ClientInfo[T], tag Optional[string], inReplyTo Optional[float64], body any) (float64, error) {
	id, err := info.ObtainId(ctx)
	if err != nil {
		return 0, err
	}
	header := MessageHeader{
		Id:        id,
		InReplyTo: inReplyTo,
		Tag:       tag,
	}
	return header.Id, info.Send(ctx, header, body)
}

func (c *ClientInfo[T]) StartRequestThread[R any](ctx context.Context,
	threadTimeout time.Duration, tag string, body any,
	handlerFn func(hd *HandlerData[T], ctx context.Context,
		receiver Receiver) (R, error),
) (R, error) {
	threadCtx := context.WithValue(ctx, ctxlog.TagKey, tag)
	return c.startThread(threadCtx, threadTimeout, func(handlerData *HandlerData[T], ctx context.Context) (R, error) {
		handlerData.sendOnThread(ctx, body, nil, Optional[float64]{}, Some(tag))
		ret, err := handlerFn(handlerData, ctx, MakeReceiver(handlerData))
		if err == nil {
			ctxlog.Printf(ctx, "Client handled request successfully")
		}
		return ret, err
	})
}

func (c *ClientInfo[T]) CloseDueToError(ctx context.Context, err error) {
	if closeError, ok := err.(websocket.CloseError); ok {
		closeWithError(ctx, closeError.Code, closeError.Reason, c)
	} else {
		closeWithError(ctx, websocket.StatusInternalError, err.Error(), c)
	}
}

func (c *ClientInfo[T]) StartRequestThreadCloseOnError[R any](ctx context.Context,
	threadTimeout time.Duration, tag string, body any,
	handlerFn func(hd *HandlerData[T], ctx context.Context,
		receiver Receiver) (R, error),
) (R, error) {
	val, err := c.StartRequestThread(ctx, threadTimeout, tag, body, handlerFn)
	if err != nil {
		c.CloseDueToError(ctx, err)
		return Zero[R](), err
	}
	return val, nil
}

func (h *Hub[T]) Register(ctx context.Context, c *ClientInfo[T]) error {
	err1, err2 := h.lb.WriteReturning(ctx, func(value *HubContent[T]) error {
		if value.isClosed {
			return &HubIsClosedErrorInstance
		}
		ctxlog.Printf(ctx, "Register client")
		value.clients[c] = true
		return nil
	})
	if err2 != nil {
		return err2
	}
	return err1
}

func (h *Hub[T]) Unregister(ctx context.Context, item UnregisterItem[T]) error {
	ctx = context.WithValue(ctx, ctxlog.ClientIdKey, item.info.Id)
	onUnregistered, err1, err2 := h.lb.WriteReturning2(ctx, func(value *HubContent[T]) (func(UnregisterItem[T]), error) {
		if value.isClosed {
			return nil, &HubIsClosedErrorInstance
		}
		delete(value.clients, item.info)
		item.info.conn.Close(item.status, item.reason)
		ctxlog.Printf(ctx, "Close client, status=%v, reason=%v", item.status, item.reason)
		return value.On.Unregistered, nil
	})
	if err2 != nil {
		return err2
	}
	if err1 != nil {
		return err1
	}
	if onUnregistered != nil {
		onUnregistered(item)
	}
	return nil
}

func (h *Hub[T]) Close(ctx context.Context) error {
	err1, err2 := h.lb.WriteReturning(ctx, func(value *HubContent[T]) error {
		if value.isClosed {
			return &HubIsClosedErrorInstance
		}
		value.isClosed = true
		ctxlog.Printf(ctx, "Close hub")
		select {
		case h.closed <- VVoid:
		default:
		}
		for c := range value.clients {
			c.conn.CloseNow()
		}
		return nil
	})
	if err2 != nil {
		return err2
	}
	return err1
}

func (h *Hub[T]) Closed() <-chan Void {
	return h.closed
}

func (h *Hub[T]) ThreadTimeout(ctx context.Context) (time.Duration, error) {
	return h.lb.ReadReturning(ctx, func(value *HubContent[T]) time.Duration {
		return value.ThreadTimeout
	})
}

// Exactly one of Tag and InReplyTo should be zero
type MessageHeader struct {
	InReplyTo Optional[float64] `json:"inReplyTo"`
	Tag       Optional[string]  `json:"tag"`
	Id        float64           `json:"id"`
}

type HandlerData[T any] struct {
	hub  *Hub[T]
	info *ClientInfo[T]
	// (data, id), received by us
	in      chan Pair[[]byte, float64]
	threads *Lockbox[OpenThreads[T]]
	TraceId int32
}

type (
	Responder func(ctx context.Context, body any) (Receiver, error)
	Receiver  func(context.Context) ([]byte, Responder, error)
)

func MakeSender[T any](replyingTo float64, handlerData *HandlerData[T]) Responder {
	used := false
	return func(ctx context.Context, data any) (Receiver, error) {
		if used {
			return nil, fmt.Errorf("Sender was used already")
		}
		used = true
		handlerData.sendOnThread(ctx, data, nil, Some(replyingTo), Optional[string]{})
		return MakeReceiver(handlerData), nil
	}
}

func MakeReceiver[T any](handlerData *HandlerData[T]) Receiver {
	used := false
	return func(ctx context.Context) ([]byte, Responder, error) {
		if used {
			return nil, nil, fmt.Errorf("Receiver was used already")
		}
		used = true
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case event := <-handlerData.in:
			return event.First, MakeSender(event.Second, handlerData), nil
		}
	}
}

type HandlerFunc[T any] func(hd *HandlerData[T], ctx context.Context, req []byte, responder Responder) error

// Returns error with ctx expires
func (h *Hub[T]) RegisterHandler(ctx context.Context, tag string, handler HandlerFunc[T]) error {
	err1, err2 := h.lb.WriteReturning(ctx, func(value *HubContent[T]) error {
		if value.isClosed {
			return &HubIsClosedErrorInstance
		}
		_, tagExistsAlready := value.handlers[tag]
		if tagExistsAlready {
			return fmt.Errorf("Handler for tag %v exists already", tag)
		}
		value.handlers[tag] = handler
		return nil
	})
	if err2 != nil {
		return err2
	}
	return err1
}

func (info *ClientInfo[T]) Send(ctx context.Context, header MessageHeader, body any) error {
	sHeader, err := json.Marshal(header)
	if err != nil {
		return err
	}
	_ = sHeader
	sBody, err := json.Marshal(body)
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
	return nil
}

func (hd *HandlerData[T]) sendOnThread(ctx context.Context, body any, err error, inReplyTo Optional[float64], tag Optional[string]) error {
	if err != nil {
		protErr, ok := err.(websocket.CloseError)
		if ok {
			closeWithError(ctx, protErr.Code, protErr.Reason, hd.info)
		} else {
			closeWithError(ctx, websocket.StatusInternalError, err.Error(), hd.info)
		}
		return nil
	}

	id, err := SendMessageToClient(ctx, hd.info, tag, inReplyTo, body)
	if err != nil {
		return err
	}

	return hd.threads.Write(ctx, func(t *OpenThreads[T]) {
		t.msgIdToHandlerData[id] = hd
		t.handlerDataToMsgId[hd] = id
	})
}

func closeWithError[T any](ctx context.Context, status websocket.StatusCode, reason string, info *ClientInfo[T]) {
	if info.isUnregistered.Swap(true) {
		return
	}
	if len(reason) > 125 {
		reason = reason[:125]
	}
	info.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := info.hub.Unregister(ctx, UnregisterItem[T]{info, status, reason}); err != nil {
		ctxlog.Printf(ctx, "Could not unregister client: %v", err)
	}
	cancel()
}

type OpenThreads[T any] struct {
	msgIdToHandlerData map[float64]*HandlerData[T]
	handlerDataToMsgId map[*HandlerData[T]]float64
}

func (c *ClientInfo[T]) startThread[R any](ctx context.Context,
	timeout time.Duration, handlerFn func(handlerData *HandlerData[T],
		ctx context.Context) (R, error),
) (R, error) {
	handlerData := &HandlerData[T]{
		hub:     c.hub,
		info:    c,
		in:      make(chan Pair[[]byte, float64]),
		threads: &c.OpenThreads,
		TraceId: lastTraceId.Add(1),
	}
	defer func() {
		clientCtx := context.WithValue(c.ctx, ctxlog.TraceIdKey, handlerData.TraceId)
		cleanUpCtx, cleanUpCancel := context.WithTimeout(clientCtx, time.Second)
		cleanUpErr := c.OpenThreads.Write(cleanUpCtx, func(t *OpenThreads[T]) {
			id := t.handlerDataToMsgId[handlerData]
			l1 := len(t.handlerDataToMsgId)
			delete(t.handlerDataToMsgId, handlerData)
			if len(t.handlerDataToMsgId) < l1 {
				delete(t.msgIdToHandlerData, id)
			}
		})
		if cleanUpErr != nil {
			ctxlog.Printf(ctx, "Could not delete thread data from ClientInfo: %v\n", cleanUpErr)
		}
		cleanUpCancel()
	}()

	ctx3, cancel3 := context.WithTimeout(
		context.WithValue(ctx, ctxlog.TraceIdKey, handlerData.TraceId),
		timeout,
	)
	ctx3 = context.WithValue(ctx3, ctxlog.ClientIdKey, c.Id)
	ret, err := handlerFn(handlerData, ctx3)
	cancel3()
	return ret, err
}

func handleClientTaskQueue[T any](ctx context.Context, info *ClientInfo[T], timeout time.Duration) {
	for {
		var task ClientTask[T]
		select {
		case <-ctx.Done():
			return
		case task = <-info.Tasks:
		}
		taskCtx := context.WithoutCancel(task.ctx)
		taskCtx, cancelTask := DerivedFromTwoContexts(taskCtx, ctx)
		taskCtx = context.WithValue(taskCtx, ctxlog.ClientTaskKey, task.name)
		taskCtx2, cancelTask2 := context.WithTimeout(taskCtx, timeout)
		err := task.f(taskCtx2, info)
		cancelTask()
		cancelTask2()
		if err != nil {
			ctxlog.Printf(taskCtx, "Could not execute client task: %v", err.Error())
		} else {
			ctxlog.Printf(taskCtx, "Client task executed successfully")
		}
	}
}

func handleWS[T any](hub *Hub[T], w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:5173", "org.thymewatch://"},
	})
	if err != nil {
		ctxlog.Printf(ctx, "accept failed: %v", err)
		return
	}

	clientId := lastClientId.Add(1)
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), ctxlog.ClientIdKey, clientId))

	isUnregistered := atomic.Bool{}

	info := &ClientInfo[T]{
		Id:             clientId,
		hub:            hub,
		conn:           conn,
		isUnregistered: &isUnregistered,
		OpenThreads: MakeLockbox(OpenThreads[T]{
			msgIdToHandlerData: make(map[float64]*HandlerData[T]),
			handlerDataToMsgId: make(map[*HandlerData[T]]float64),
		}),
		currId: MakeLockbox(0.0),
		ctx:    ctx,
		cancel: cancel,
		Tasks:  make(chan ClientTask[T], 100),
	}

	ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
	hub.Register(ctx2, info)
	cancel2()
	closeWithError := func(status websocket.StatusCode, reason string) {
		ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
		closeWithError(ctx2, status, reason, info)
		cancel2()
	}
	defer closeWithError(websocket.StatusNormalClosure, "")
	taskTimeout, err := hub.lb.ReadReturning(ctx, func(value *HubContent[T]) time.Duration {
		return value.TaskTimeout
	})
	if err != nil {
		return
	}
	go handleClientTaskQueue(ctx, info, taskTimeout)

	for {
		ctx2, cancel2 := context.WithCancel(ctx)
		_, msg, err := conn.Read(ctx2)
		cancel2()
		if err != nil {
			ctxlog.Printf(ctx, "Could not read from client: %v", err)
			closeWithError(websocket.StatusInternalError, "Could not read from client")
			return
		}

		endHeader := slices.Index(msg, '\n')
		if endHeader == -1 {
			ctxlog.Printf(ctx, "Receiver message without header")
			closeWithError(websocket.StatusProtocolError, "Message does not start with header")
			return
		}
		header := MessageHeader{}
		err = json.Unmarshal(msg[:endHeader], &header)
		if err != nil {
			ctxlog.Printf(ctx, "Receiver invalid header: %v", err)
			closeWithError(websocket.StatusProtocolError, "Invalid header")
		}
		if header.Tag.present {
			ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
			handlerFn, threadTimeout, err := hub.lb.ReadReturning2(ctx2, func(value *HubContent[T]) (HandlerFunc[T], time.Duration) {
				return value.handlers[header.Tag.value], value.ThreadTimeout
			})
			cancel2()
			if err != nil {
				ctxlog.Printf(ctx2, "Could not access hub data: %v", err)
				closeWithError(websocket.StatusInternalError, err.Error())
				return
			}
			if handlerFn == nil {
				ctxlog.Printf(ctx2, "Client send invalid tag \"%v\"", header.Tag.value)
				closeWithError(websocket.StatusProtocolError, "Invalid tag")
				return
			}
			handlerCtx := context.WithValue(ctx, ctxlog.TagKey, header.Tag.value)
			go info.startThread(handlerCtx, threadTimeout,
				func(handlerData *HandlerData[T], ctx context.Context) (Void, error) {
					err := handlerFn(handlerData, ctx, msg[endHeader+1:],
						MakeSender(header.Id, handlerData))
					if err != nil {
						info.CloseDueToError(ctx, err)
					} else {
						ctxlog.Printf(ctx, "Request handled successfully")
					}
					return VVoid, err
				})
		} else {
			ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
			data, err := info.OpenThreads.WriteReturning(ctx2, func(t *OpenThreads[T]) *HandlerData[T] {
				data := t.msgIdToHandlerData[header.InReplyTo.value]
				delete(t.msgIdToHandlerData, header.InReplyTo.value)
				delete(t.handlerDataToMsgId, data)
				return data
			})
			cancel2()
			if err != nil {
				ctxlog.Printf(ctx2, "Could not update ClientInfo.OpenThreads: %v", err)
				closeWithError(websocket.StatusInternalError, err.Error())
				return
			}
			if data == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case data.in <- MakePair(msg[endHeader+1:], header.Id):
			}
		}
	}
}
