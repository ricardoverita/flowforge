package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Broker struct {
	Conn *nats.Conn
	JS   nats.JetStreamContext
}

func Open(url, token, service string, logger *slog.Logger) (*Broker, error) {
	opts := []nats.Option{nats.Name(service), nats.Timeout(5 * time.Second), nats.MaxReconnects(-1), nats.ReconnectWait(time.Second), nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
		logger.Error("NATS asynchronous operation failed", "error", err)
	})}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		nc.Close()
		return nil, err
	}
	b := &Broker{Conn: nc, JS: js}
	if err = b.ensure(); err != nil {
		nc.Close()
		return nil, err
	}
	return b, nil
}
func (b *Broker) ensure() error {
	for _, config := range []*nats.StreamConfig{
		{Name: "FLOWFORGE_TASKS", Subjects: []string{TaskPrefix + ">"}, Storage: nats.FileStorage, Retention: nats.WorkQueuePolicy, MaxBytes: 256 << 20, MaxMsgSize: 1 << 21, Discard: nats.DiscardNew, Duplicates: 2 * time.Minute},
		{Name: "FLOWFORGE_RESULTS", Subjects: []string{ResultSubject}, Storage: nats.FileStorage, Retention: nats.WorkQueuePolicy, MaxBytes: 256 << 20, MaxMsgSize: 1 << 21, Discard: nats.DiscardNew, Duplicates: 2 * time.Minute},
	} {
		_, err := b.JS.StreamInfo(config.Name)
		if errors.Is(err, nats.ErrStreamNotFound) {
			_, err = b.JS.AddStream(config)
			if err != nil {
				if _, e := b.JS.StreamInfo(config.Name); e == nil {
					continue
				}
			}
		}
		if err != nil {
			return fmt.Errorf("ensure stream %s: %w", config.Name, err)
		}
	}
	return nil
}
func (b *Broker) Ready(ctx context.Context) error {
	if !b.Conn.IsConnected() {
		return fmt.Errorf("NATS is disconnected")
	}
	_, err := b.JS.AccountInfo(nats.Context(ctx))
	return err
}
func (b *Broker) Close() { _ = b.Conn.Drain(); b.Conn.Close() }
func (b *Broker) Publish(ctx context.Context, subject, id string, data []byte, traceparent string) error {
	if traceparent != "" {
		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{"traceparent": traceparent})
	}
	ctx, span := otel.Tracer("flowforge/messaging").Start(ctx, "nats.publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	msg := nats.NewMsg(subject)
	msg.Data = data
	msg.Header.Set(nats.MsgIdHdr, id)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		msg.Header.Set(k, v)
	}
	_, err := b.JS.PublishMsg(msg, nats.Context(ctx))
	return err
}
func (b *Broker) Subscribe(subject, durable, stream string) (*nats.Subscription, error) {
	return b.JS.PullSubscribe(subject, durable, nats.BindStream(stream), nats.ManualAck(), nats.AckExplicit(), nats.AckWait(30*time.Second), nats.MaxAckPending(128))
}
func MessageContext(ctx context.Context, msg *nats.Msg) context.Context {
	carrier := propagation.MapCarrier{}
	for k, v := range msg.Header {
		if len(v) > 0 {
			carrier[k] = v[0]
		}
	}
	// Header canonicalization is case-insensitive; MapCarrier is not.
	carrier.Set("traceparent", msg.Header.Get("traceparent"))
	carrier.Set("tracestate", msg.Header.Get("tracestate"))
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
