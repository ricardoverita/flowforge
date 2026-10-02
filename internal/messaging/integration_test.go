//go:build integration

package messaging

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestJetStreamDuplicatePublishAndRedelivery(t *testing.T) {
	url := os.Getenv("TEST_NATS_URL")
	if url == "" {
		t.Skip("TEST_NATS_URL required; run make test-integration")
	}
	b, err := Open(url, os.Getenv("TEST_NATS_TOKEN"), "integration", slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	name := "TEST_" + uuid.NewString()[:8]
	subject := "test." + name
	_, err = b.JS.AddStream(&nats.StreamConfig{Name: name, Subjects: []string{subject}, Storage: nats.FileStorage, Duplicates: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.JS.DeleteStream(name) }()
	sub, err := b.JS.PullSubscribe(subject, "test-consumer", nats.BindStream(name), nats.ManualAck(), nats.AckExplicit(), nats.AckWait(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	ctx := context.Background()
	for range 2 {
		if err = b.Publish(ctx, subject, "same-id", []byte(`{}`), ""); err != nil {
			t.Fatal(err)
		}
	}
	info, err := b.JS.StreamInfo(name)
	if err != nil || info.State.Msgs != 1 {
		t.Fatal("publish not deduplicated", err)
	}
	msgs, err := sub.Fetch(1, nats.MaxWait(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := msgs[0].Metadata()
	sequence := meta.Sequence.Stream
	msgs, err = sub.Fetch(1, nats.MaxWait(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	meta, _ = msgs[0].Metadata()
	if meta.Sequence.Stream != sequence || meta.NumDelivered < 2 {
		t.Fatal("unacknowledged message was not redelivered")
	}
	if err = msgs[0].AckSync(); err != nil {
		t.Fatal(err)
	}
	_, err = sub.Fetch(1, nats.MaxWait(300*time.Millisecond))
	if !errors.Is(err, nats.ErrTimeout) {
		t.Fatal("acknowledged message redelivered", err)
	}
}
