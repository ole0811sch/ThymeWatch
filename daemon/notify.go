package main

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	// used to signal that from within this program the notification was closed
	NotificationClosedReasonInternal NotificationClosedReason = iota
	NotificationClosedReasonExpired
	NotificationClosedReasonDismissed
	NotificationClosedReasonCall
	NotificationClosedReasonUndefined
)

type NotificationClosedReason int

// Sends notification, calls onSent once it is sent and returns when it is
// closed. This call may continue past the lifetime of ctx, which isn't
// respected after the notification has been sent. It should be canceled within
// onSent. If at any point something is sent on close, the notification will be
// closed.
func Notify(ctx context.Context, title string, body string, onSent func(id uint32), close <-chan Void) (NotificationClosedReason, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return 1, err
	}
	defer conn.Close()

	if err = conn.AddMatchSignalContext(
		ctx,
		dbus.WithMatchObjectPath("/org/freedesktop/Notifications"),
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchMember("NotificationClosed"),
	); err != nil {
		return 1, err
	}
	c := make(chan *dbus.Signal, 10)
	conn.Signal(c)
	defer conn.RemoveSignal(c)

	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.CallWithContext(ctx, "org.freedesktop.Notifications.Notify", 0, "", uint32(0),
		"", title, body, []string{},
		map[string]dbus.Variant{
			"urgency":        dbus.MakeVariant(2),
			"suppress-sound": dbus.MakeVariant(true),
		}, int32(0))
	if call.Err != nil {
		return 1, call.Err
	}
	var id uint32
	err = call.Store(&id)
	if err != nil {
		return 1, err
	}
	onSent(id)

	for {
		var v *dbus.Signal
		var ok bool
		select {
		case v, ok = <-c:
			if !ok {
				return 1, fmt.Errorf("Can't read signals from dbus")
			}
		case <-close:
			err := closeNotification(obj, id)
			if err != nil {
				return 1, err
			}
			return NotificationClosedReasonInternal, nil
		}
		if v.Name != "org.freedesktop.Notifications.NotificationClosed" {
			continue
		}
		if len(v.Body) < 2 {
			return 1, fmt.Errorf("NotificationClosed signal's body is too short")
		}
		id2, okId := v.Body[0].(uint32)
		reason, okReason := v.Body[1].(uint32)
		if !okId || !okReason || reason < uint32(NotificationClosedReasonExpired) || reason > uint32(NotificationClosedReasonUndefined) {
			return 1, fmt.Errorf("NotificationClosed signal's body does not match the expected format")
		}
		if id2 != id {
			continue
		}
		return NotificationClosedReason(reason), nil
	}
}

// Uses a timeout of 5 seconds
func closeNotification(obj dbus.BusObject, id uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	call := obj.CallWithContext(ctx, "org.freedesktop.Notifications.CloseNotification", 0, id)
	cancel()
	return call.Err
}
