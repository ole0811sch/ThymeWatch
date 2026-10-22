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
	idChan         <-chan float64
	outChan        chan OutChanItem
	conn           *websocket.Conn
	isUnregistered *atomic.Bool
	cancel         context.CancelFunc
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

// returns id, ctx.Err()
func SendMessageToClient[T any](ctx context.Context, info *ClientInfo[T], tag Optional[string], inReplyTo Optional[float64], body any) (float64, error) {
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
			handlerData.sendOnThread(ctx, nil, data.(error), replyingTo)
			return nil, nil
		} else {
			handlerData.sendOnThread(ctx, data, nil, replyingTo)
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

func handleOutgoingMessages[T any](ctx context.Context, info *ClientInfo[T]) {
	err := func() error {
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

func (hd *HandlerData[T]) sendOnThread(ctx context.Context, body any, err error, inReplyTo float64) {
	if err != nil {
		protErr, ok := err.(websocket.CloseError)
		if ok {
			closeWithError(protErr.Code, protErr.Reason, hd.info)
		} else {
			closeWithError(websocket.StatusInternalError, err.Error(), hd.info)
		}
		return
	}

	id, err := SendMessageToClient(ctx, hd.info, Optional[string]{}, Some(inReplyTo), body)
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
	outChan := make(chan OutChanItem)
	isUnregistered := atomic.Bool{}
	info := &ClientInfo[T]{
		hub:            hub,
		idChan:         idChan,
		outChan:        outChan,
		conn:           conn,
		isUnregistered: &isUnregistered,
		cancel:         cancel,
	}
	go handleIdDistribution(ctx, idChan)
	go handleOutgoingMessages(ctx, info)

	hub.Register(info)
	closeWithError := func(status websocket.StatusCode, reason string) {
		closeWithError(status, reason, info)
	}
	defer closeWithError(websocket.StatusNormalClosure, "")
	openThreads := MakeLockbox(OpenThreads[T]{
		msgIdToHandlerData: make(map[float64]*HandlerData[T]),
		handlerDataToMsgId: make(map[*HandlerData[T]]float64),
	})

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
			go func() {
				handlerData := &HandlerData[T]{
					hub:     hub,
					info:    info,
					in:      make(chan Pair[[]byte, float64]),
					threads: &openThreads,
				}
				ctx3, cancel3 := context.WithTimeout(ctx, threadTimeout)
				handlerFn(handlerData, ctx3, msg[endHeader+1:], MakeSender(header.Id, handlerData))
				cancel3()
				openThreads.Write(func(t *OpenThreads[T]) {
					id := t.handlerDataToMsgId[handlerData]
					l1 := len(t.handlerDataToMsgId)
					delete(t.handlerDataToMsgId, handlerData)
					if len(t.handlerDataToMsgId) < l1 {
						delete(t.msgIdToHandlerData, id)
					}
				})
			}()
		} else {
			data := openThreads.WriteReturning(func(t *OpenThreads[T]) *HandlerData[T] {
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
