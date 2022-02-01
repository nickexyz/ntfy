package server

import (
	"context"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"fmt"
	"google.golang.org/api/option"
	"heckel.io/ntfy/auth"
	"strings"
)

type firebaseSender func(m *messaging.Message) error

func createFirebaseSender(conf *Config) (firebaseSender, error) {
	fb, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile(conf.FirebaseKeyFile))
	if err != nil {
		return nil, err
	}
	msg, err := fb.Messaging(context.Background())
	if err != nil {
		return nil, err
	}
	return func(m *messaging.Message) error {
		_, err := msg.Send(context.Background(), m)
		return err
	}, nil
}

func createFirebaseSubscriber(auther auth.Auther, sender firebaseSender) (subscriber, error) {
	return func(m *message) error {
		var data map[string]string // Mostly matches https://ntfy.sh/docs/subscribe/api/#json-message-format
		switch m.Event {
		case keepaliveEvent, openEvent:
			data = map[string]string{
				"id":    m.ID,
				"time":  fmt.Sprintf("%d", m.Time),
				"event": m.Event,
				"topic": m.Topic,
			}
		case messageEvent:
			allowForward := true
			if auther != nil {
				allowForward = auther.Authorize(nil, m.Topic, auth.PermissionRead) == nil
			}
			if allowForward {
				data = map[string]string{
					"id":       m.ID,
					"time":     fmt.Sprintf("%d", m.Time),
					"event":    m.Event,
					"topic":    m.Topic,
					"priority": fmt.Sprintf("%d", m.Priority),
					"tags":     strings.Join(m.Tags, ","),
					"click":    m.Click,
					"title":    m.Title,
					"message":  m.Message,
					"encoding": m.Encoding,
				}
				if m.Attachment != nil {
					data["attachment_name"] = m.Attachment.Name
					data["attachment_type"] = m.Attachment.Type
					data["attachment_size"] = fmt.Sprintf("%d", m.Attachment.Size)
					data["attachment_expires"] = fmt.Sprintf("%d", m.Attachment.Expires)
					data["attachment_url"] = m.Attachment.URL
				}
			} else {
				// If anonymous read for a topic is not allowed, we cannot send the message along
				// via Firebase. Instead, we send a "poll_request" message, asking the client to poll.
				data = map[string]string{
					"id":    m.ID,
					"time":  fmt.Sprintf("%d", m.Time),
					"event": pollRequestEvent,
					"topic": m.Topic,
				}
			}
		}
		var androidConfig *messaging.AndroidConfig
		if m.Priority >= 4 {
			androidConfig = &messaging.AndroidConfig{
				Priority: "high",
			}
		}
		return sender(maybeTruncateFCMMessage(&messaging.Message{
			Topic:   m.Topic,
			Data:    data,
			Android: androidConfig,
		}))
	}, nil
}