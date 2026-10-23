package main

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type HubContent[T any] struct {
	handlers map[string]HandlerFunc[T]
	clients  map[*ClientInfo[T]]bool
	// event handlers may still be called shortly after they have removed
	On struct {
		Unregistered func(e UnregisterItem[T])
	}
	// negative for no timeout
	ThreadTimeout time.Duration
	isClosed      bool
}

// UserData can be accessed/modified without obtaining a lock (it is assumed to be synchronized internally)
type Hub[T any] struct {
	lb       Lockbox[HubContent[T]]
	UserData T
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

type ClientInfo[T any] struct {
	hub            *Hub[T]
	currId         Lockbox[float64]
	conn           *websocket.Conn
	isUnregistered *atomic.Bool
	cancel         context.CancelFunc
	OpenThreads    Lockbox[OpenThreads[T]]
}

func (c *ClientInfo[T]) ObtainId() float64 {
	return c.currId.WriteReturning(func(value *float64) float64 {
		ret := (*value)
		*value++
		return ret
	})
}

type HubIsClosedError struct{}

var HubIsClosedErrorInstance = HubIsClosedError{}

func (e *HubIsClosedError) Error() string {
	return "The hub is closed already"
}

func newHub[T any](userData T) *Hub[T] {
	return &Hub[T]{
		lb: Lockbox[HubContent[T]]{
			value: HubContent[T]{
				handlers:      make(map[string]HandlerFunc[T]),
				clients:       make(map[*ClientInfo[T]]bool),
				ThreadTimeout: -1,
			},
		},
		UserData: userData,
	}
}

func Some[T any](value T) Optional[T] {
	return Optional[T]{value: value, present: true}
}

func SendMessageToClient[T any](ctx context.Context, info *ClientInfo[T], tag Optional[string], inReplyTo Optional[float64], body any) (float64, error) {
	header := MessageHeader{
		Id:        info.ObtainId(),
		InReplyTo: inReplyTo,
		Tag:       tag,
	}
	return header.Id, info.Send(ctx, header, body)
}

func (c *ClientInfo[T]) StartRequestThread[R any](ctx context.Context,
	threadTimeout time.Duration, tag string, body any,
	handlerFn func(hd *HandlerData[T], ctx context.Context,
		receiver Receiver) R,
) R {
	return c.startThread(ctx, threadTimeout, func(handlerData *HandlerData[T], ctx context.Context) R {
		handlerData.sendOnThread(ctx, body, nil, Optional[float64]{}, Some(tag))
		return handlerFn(handlerData, ctx, MakeReceiver(handlerData))
	})
}

func (h *Hub[T]) Register(c *ClientInfo[T]) error {
	return h.lb.WriteReturning(func(value *HubContent[T]) error {
		if value.isClosed {
			return &HubIsClosedErrorInstance
		}
		fmt.Println("Hub: register")
		value.clients[c] = true
		return nil
	})
}

func (h *Hub[T]) Unregister(item UnregisterItem[T]) error {
	onUnregistered, err := h.lb.WriteReturning2(func(value *HubContent[T]) (func(UnregisterItem[T]), error) {
		if value.isClosed {
			return nil, &HubIsClosedErrorInstance
		}
		fmt.Println("Hub: unregister")
		delete(value.clients, item.info)
		item.info.conn.Close(item.status, item.reason)
		return value.On.Unregistered, nil
	})
	if err != nil {
		return err
	}
	if onUnregistered != nil {
		onUnregistered(item)
	}
	return nil
}

func (h *Hub[T]) Close() error {
	return h.lb.WriteReturning(func(value *HubContent[T]) error {
		if value.isClosed {
			return &HubIsClosedErrorInstance
		}
		value.isClosed = true
		fmt.Println("Hub: close")
		for c := range value.clients {
			c.conn.CloseNow()
		}
		return nil
	})
}

func (h *Hub[T]) ThreadTimeout() time.Duration {
	return h.lb.ReadReturning(func(value *HubContent[T]) time.Duration {
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
}

type (
	Responder func(context.Context, any, bool) (Receiver, error)
	Receiver  func(context.Context) ([]byte, Responder, error)
)

func MakeSender[T any](replyingTo float64, handlerData *HandlerData[T]) Responder {
	used := false
	return func(ctx context.Context, data any, isErr bool) (Receiver, error) {
		if used {
			return nil, fmt.Errorf("Sender was used already")
		}
		used = true
		if isErr {
			handlerData.sendOnThread(ctx, nil, data.(error), Some(replyingTo), Optional[string]{})
			return nil, nil
		} else {
			handlerData.sendOnThread(ctx, data, nil, Some(replyingTo), Optional[string]{})
			return MakeReceiver(handlerData), nil
		}
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

type HandlerFunc[T any] func(hd *HandlerData[T], ctx context.Context, req []byte, responder Responder)

func (h *Hub[T]) RegisterHandler(tag string, handler HandlerFunc[T]) error {
	return h.lb.WriteReturning(func(value *HubContent[T]) error {
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

func (hd *HandlerData[T]) sendOnThread(ctx context.Context, body any, err error, inReplyTo Optional[float64], tag Optional[string]) {
	if err != nil {
		protErr, ok := err.(websocket.CloseError)
		if ok {
			closeWithError(protErr.Code, protErr.Reason, hd.info)
		} else {
			closeWithError(websocket.StatusInternalError, err.Error(), hd.info)
		}
		return
	}

	id, err := SendMessageToClient(ctx, hd.info, tag, inReplyTo, body)
	if err != nil {
		return // ctx is done
	}

	hd.threads.Write(func(t *OpenThreads[T]) {
		t.msgIdToHandlerData[id] = hd
		t.handlerDataToMsgId[hd] = id
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

func closeWithError[T any](status websocket.StatusCode, reason string, info *ClientInfo[T]) {
	if info.isUnregistered.Swap(true) {
		return
	}
	if len(reason) > 125 {
		reason = reason[:125]
	}
	info.hub.Unregister(UnregisterItem[T]{info, status, reason})
	info.cancel()
}

type OpenThreads[T any] struct {
	msgIdToHandlerData map[float64]*HandlerData[T]
	handlerDataToMsgId map[*HandlerData[T]]float64
}

func (c *ClientInfo[T]) startThread[R any](ctx context.Context, timeout time.Duration, handlerFn func(handlerData *HandlerData[T], ctx context.Context) R) R {
	handlerData := &HandlerData[T]{
		hub:     c.hub,
		info:    c,
		in:      make(chan Pair[[]byte, float64]),
		threads: &c.OpenThreads,
	}
	ctx3, cancel3 := context.WithTimeout(
		ctx,
		timeout,
	)
	ret := handlerFn(handlerData, ctx3)
	cancel3()
	c.OpenThreads.Write(func(t *OpenThreads[T]) {
		id := t.handlerDataToMsgId[handlerData]
		l1 := len(t.handlerDataToMsgId)
		delete(t.handlerDataToMsgId, handlerData)
		if len(t.handlerDataToMsgId) < l1 {
			delete(t.msgIdToHandlerData, id)
		}
	})
	return ret
}

func handleWS[T any](hub *Hub[T], w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:5173"},
	})
	if err != nil {
		log.Printf("accept failed: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())

	idChan := make(chan float64)
	isUnregistered := atomic.Bool{}

	info := &ClientInfo[T]{
		hub:            hub,
		conn:           conn,
		isUnregistered: &isUnregistered,
		OpenThreads: MakeLockbox(OpenThreads[T]{
			msgIdToHandlerData: make(map[float64]*HandlerData[T]),
			handlerDataToMsgId: make(map[*HandlerData[T]]float64),
		}),
		cancel: cancel,
	}
	go handleIdDistribution(ctx, idChan)

	hub.Register(info)
	closeWithError := func(status websocket.StatusCode, reason string) {
		closeWithError(status, reason, info)
	}
	defer closeWithError(websocket.StatusNormalClosure, "")

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
			handlerFn, threadTimeout := hub.lb.ReadReturning2(func(value *HubContent[T]) (HandlerFunc[T], time.Duration) {
				return value.handlers[header.Tag.value], value.ThreadTimeout
			})
			if handlerFn == nil {
				fmt.Printf("Client send invalid tag \"%v\"\n", header.Tag.value)
				closeWithError(websocket.StatusProtocolError, "Invalid tag")
				return
			}
			go info.startThread(ctx, threadTimeout, func(handlerData *HandlerData[T], ctx context.Context) struct{} {
				handlerFn(handlerData, ctx, msg[endHeader+1:], MakeSender(header.Id, handlerData))
				return struct{}{}
			})
		} else {
			data := info.OpenThreads.WriteReturning(func(t *OpenThreads[T]) *HandlerData[T] {
				data := t.msgIdToHandlerData[header.InReplyTo.value]
				delete(t.msgIdToHandlerData, header.InReplyTo.value)
				delete(t.handlerDataToMsgId, data)
				return data
			})
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
