package tests

import (
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type fakeToken struct {
	waitResult bool
	err        error
	done       chan struct{}
}

func (t *fakeToken) Wait() bool {
	return t.waitResult
}

func (t *fakeToken) WaitTimeout(_ time.Duration) bool {
	return t.waitResult
}

func (t *fakeToken) Done() <-chan struct{} {
	if t.done == nil {
		t.done = make(chan struct{})
		close(t.done)
	}
	return t.done
}

func (t *fakeToken) Error() error {
	return t.err
}

type fakeMQTTClient struct {
	connectToken   mqtt.Token
	publishToken   mqtt.Token
	subscribeToken mqtt.Token
	disconnects    int
	publishedTopic string
	publishedQos   byte
	publishedRet   bool
	payload        interface{}
	subscribedTo   string
	subscribeBody  string
}

func (c *fakeMQTTClient) IsConnected() bool { return true }

func (c *fakeMQTTClient) IsConnectionOpen() bool { return true }

func (c *fakeMQTTClient) Connect() mqtt.Token {
	if c.connectToken != nil {
		return c.connectToken
	}
	return &fakeToken{waitResult: true}
}

func (c *fakeMQTTClient) Disconnect(_ uint) { c.disconnects++ }

func (c *fakeMQTTClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	c.publishedTopic = topic
	c.publishedQos = qos
	c.publishedRet = retained
	c.payload = payload
	if c.publishToken == nil {
		return &fakeToken{waitResult: true}
	}
	return c.publishToken
}

func (c *fakeMQTTClient) Subscribe(topic string, _ byte, callback mqtt.MessageHandler) mqtt.Token {
	c.subscribedTo = topic
	if callback != nil && c.subscribeBody != "" {
		callback(c, &fakeMessage{topic: topic, payload: []byte(c.subscribeBody)})
	}
	if c.subscribeToken == nil {
		return &fakeToken{waitResult: true}
	}
	return c.subscribeToken
}

func (c *fakeMQTTClient) SubscribeMultiple(_ map[string]byte, _ mqtt.MessageHandler) mqtt.Token {
	return &fakeToken{waitResult: true}
}

func (c *fakeMQTTClient) Unsubscribe(_ ...string) mqtt.Token { return &fakeToken{waitResult: true} }

func (c *fakeMQTTClient) AddRoute(_ string, _ mqtt.MessageHandler) {}

func (c *fakeMQTTClient) OptionsReader() mqtt.ClientOptionsReader { return mqtt.ClientOptionsReader{} }

type fakeMessage struct {
	topic   string
	payload []byte
}

func (m *fakeMessage) Duplicate() bool   { return false }
func (m *fakeMessage) Qos() byte         { return 0 }
func (m *fakeMessage) Retained() bool    { return false }
func (m *fakeMessage) Topic() string     { return m.topic }
func (m *fakeMessage) MessageID() uint16 { return 0 }
func (m *fakeMessage) Payload() []byte   { return m.payload }
func (m *fakeMessage) Ack()              {}
