package mailer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"

	"euromni/internal/store"
)

func TestConfigRequiresTLSOutsideDevelopment(t *testing.T) {
	t.Setenv("EUROMNI_MAIL_SMTP_ADDR", "127.0.0.1:1025")
	t.Setenv("EUROMNI_MAIL_FROM", "Atelier <commandes@example.test>")
	t.Setenv("EUROMNI_MAIL_SMTP_USERNAME", "")
	t.Setenv("EUROMNI_MAIL_SMTP_PASSWORD", "")
	t.Setenv("EUROMNI_MAIL_TLS_MODE", "none")
	t.Setenv("EUROMNI_ENVIRONMENT", "production")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("unencrypted production SMTP configuration unexpectedly succeeded")
	}
	t.Setenv("EUROMNI_ENVIRONMENT", "development")
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("load local Mailpit configuration: %v", err)
	}
	if config.TLSMode != TLSDisabled || config.From.Address != "commandes@example.test" {
		t.Fatalf("local SMTP configuration = %+v", config)
	}
}

func TestRenderOrderConfirmationEscapesCustomerContent(t *testing.T) {
	message, err := RenderOrderConfirmation(store.OrderConfirmationPayload{
		OrderNumber: "EU-123", CustomerName: "<script>alert(1)</script>", Locale: "fr-FR",
		Currency: "EUR", Subtotal: "12.5000", ShippingTotal: "6.9000", GrandTotal: "19.4000",
		Items: []store.OrderConfirmationItem{{Name: "<img src=x>", Quantity: 2, LineTotal: "12.5000"}},
	}, "client@example.test", "order-confirmation:EU-123")
	if err != nil {
		t.Fatalf("render order confirmation: %v", err)
	}
	if !strings.Contains(message.Subject, "Confirmation de commande EU-123") ||
		!strings.Contains(message.TextBody, "paiement est en attente") {
		t.Fatalf("French order confirmation subject/body = %q/%q", message.Subject, message.TextBody)
	}
	if strings.Contains(message.HTMLBody, "<script>") || strings.Contains(message.HTMLBody, "<img src=x>") ||
		!strings.Contains(message.HTMLBody, "&lt;script&gt;") || !strings.Contains(message.HTMLBody, "&lt;img") {
		t.Fatalf("HTML email did not escape customer/product content: %s", message.HTMLBody)
	}
}

func TestRenderOrderStatusEscapesCustomerAndTrackingContent(t *testing.T) {
	message, err := RenderOrderStatus(store.OrderStatusPayload{
		OrderNumber: "EU-456", CustomerName: "<script>Client</script>", Locale: "fr-FR",
		Status: "shipped", Tracking: "<img src=x>",
	}, "client@example.test", "order-status:EU-456:shipped")
	if err != nil {
		t.Fatalf("render order status notification: %v", err)
	}
	if !strings.Contains(message.Subject, "Votre commande a été expédiée EU-456") ||
		!strings.Contains(message.TextBody, "Numéro de suivi: <img src=x>") {
		t.Fatalf("French order status subject/body = %q/%q", message.Subject, message.TextBody)
	}
	if strings.Contains(message.HTMLBody, "<script>") || strings.Contains(message.HTMLBody, "<img src=x>") ||
		!strings.Contains(message.HTMLBody, "&lt;script&gt;") || !strings.Contains(message.HTMLBody, "&lt;img") {
		t.Fatalf("HTML status email did not escape customer/tracking content: %s", message.HTMLBody)
	}
}

func TestRenderOrderStatusSupportsEveryLifecycleStage(t *testing.T) {
	for status, expected := range map[string]string{
		"paid":       "Paiement reçu EU-123",
		"processing": "Votre commande est en cours de traitement EU-123",
		"prepared":   "Votre commande est préparée EU-123",
		"shipped":    "Votre commande a été expédiée EU-123",
		"delivered":  "Votre commande a été livrée EU-123",
	} {
		message, err := RenderOrderStatus(store.OrderStatusPayload{
			OrderNumber: "EU-123", Locale: "fr-FR", Status: status,
		}, "client@example.test", "event-"+status)
		if err != nil || message.Subject != expected {
			t.Errorf("render %s notification = subject %q, error %v; want %q", status, message.Subject, err, expected)
		}
	}
	if _, err := RenderOrderStatus(store.OrderStatusPayload{
		OrderNumber: "EU-123", Locale: "fr-FR", Status: "cancelled",
	}, "client@example.test", "event-cancelled"); err == nil {
		t.Fatal("unsupported lifecycle status was rendered as a customer notification")
	}
}

func TestSMTPSenderSendsMultipartMessageToLocalServer(t *testing.T) {
	address, received := startFakeSMTP(t)
	from, err := parseAddress("Atelier <orders@example.test>")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewSMTPSender(Config{
		Address: address, From: from, TLSMode: TLSDisabled,
		Environment: "development", Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("create SMTP sender: %v", err)
	}
	err = sender.Send(context.Background(), Message{
		To: "client@example.test", Subject: "Confirmation de commande EU-123",
		TextBody: "Bonjour, commande reçue.", HTMLBody: "<p>Bonjour</p>",
		Idempotency: "order-confirmation:EU-123",
	})
	if err != nil {
		t.Fatalf("send local SMTP message: %v", err)
	}
	select {
	case message := <-received:
		for _, expected := range []string{
			"To: <client@example.test>", "multipart/alternative",
			"Confirmation de commande EU-123", "Bonjour, commande re=C3=A7ue.",
			"<p>Bonjour</p>",
		} {
			if !strings.Contains(message, expected) {
				t.Errorf("SMTP message is missing %q: %s", expected, message)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SMTP message")
	}
}

func TestSMTPSenderRejectsMissingSTARTTLS(t *testing.T) {
	address, _ := startFakeSMTP(t)
	from, err := parseAddress("orders@example.test")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewSMTPSender(Config{
		Address: address, From: from, TLSMode: TLSStartTLS,
		Environment: "production", Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("create SMTP sender: %v", err)
	}
	err = sender.Send(context.Background(), Message{
		To: "client@example.test", Subject: "Test", TextBody: "Test",
		HTMLBody: "<p>Test</p>", Idempotency: "test",
	})
	if err == nil || !strings.Contains(err.Error(), "does not offer required STARTTLS") {
		t.Fatalf("SMTP without STARTTLS error = %v", err)
	}
}

func TestWorkerRetriesAndMarksSuccessfulDelivery(t *testing.T) {
	message := &store.EmailOutboxMessage{
		ID: "outbox-1", EventType: "order_confirmation",
		IdempotencyKey: "order-confirmation:EU-123", Recipient: "client@example.test",
		Payload: []byte(`{"order_number":"EU-123","customer_name":"Client","locale":"fr-FR","currency":"EUR","subtotal":"1.0000","shipping_total":"0.0000","grand_total":"1.0000","items":[{"name":"Test","quantity":1,"line_total":"1.0000"}]}`),
	}
	queue := &fakeQueue{message: message}
	sender := &fakeSender{err: errors.New("temporary SMTP failure")}
	worker, err := NewWorker(queue, sender, time.Second, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := worker.DeliverOne(context.Background())
	if !claimed || err == nil {
		t.Fatalf("failed delivery result = claimed %v, error %v", claimed, err)
	}
	if queue.retryStatus != "pending" || queue.retryAttempts != 1 || queue.retryTime.Before(time.Now()) {
		t.Fatalf("failed delivery retry state = %+v", queue)
	}

	queue = &fakeQueue{message: message}
	sender = &fakeSender{}
	worker, err = NewWorker(queue, sender, time.Second, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err = worker.DeliverOne(context.Background())
	if !claimed || err != nil || !queue.sent || sender.message.To != "client@example.test" {
		t.Fatalf("successful delivery = claimed %v, error %v, queue %+v, message %+v", claimed, err, queue, sender.message)
	}
}

func TestWorkerDeliversOrderStatusNotification(t *testing.T) {
	queue := &fakeQueue{message: &store.EmailOutboxMessage{
		ID: "outbox-status-1", EventType: "order_status",
		IdempotencyKey: "order-status:order-1:delivered", Recipient: "client@example.test",
		Payload: []byte(`{"order_number":"EU-789","customer_name":"Client","locale":"fr-FR","status":"delivered"}`),
	}}
	sender := &fakeSender{}
	worker, err := NewWorker(queue, sender, time.Second, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := worker.DeliverOne(context.Background())
	if !claimed || err != nil || !queue.sent ||
		!strings.Contains(sender.message.Subject, "Votre commande a été livrée") {
		t.Fatalf("order status delivery = claimed %v, error %v, queue %+v, message %+v", claimed, err, queue, sender.message)
	}
}

type fakeQueue struct {
	message       *store.EmailOutboxMessage
	sent          bool
	retryStatus   string
	retryTime     time.Time
	retryAttempts int
}

func (q *fakeQueue) ClaimNextEmail(context.Context, time.Duration) (*store.EmailOutboxMessage, error) {
	if q.message == nil {
		return nil, nil
	}
	message := q.message
	q.message = nil
	message.Attempts++
	return message, nil
}

func (q *fakeQueue) MarkEmailSent(context.Context, string) error {
	q.sent = true
	return nil
}

func (q *fakeQueue) RetryEmail(_ context.Context, _ string, availableAt time.Time, _ string, permanently bool) error {
	q.retryTime = availableAt
	if permanently {
		q.retryStatus = "failed"
	} else {
		q.retryStatus = "pending"
	}
	q.retryAttempts++
	return nil
}

type fakeSender struct {
	err     error
	message Message
}

func (s *fakeSender) Send(_ context.Context, message Message) error {
	s.message = message
	return s.err
}

func startFakeSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start fake SMTP listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 local test SMTP\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(strings.ToUpper(line), "EHLO "):
				_, _ = fmt.Fprint(conn, "250-localhost\r\n250 SIZE 1048576\r\n")
			case strings.HasPrefix(strings.ToUpper(line), "HELO "):
				_, _ = fmt.Fprint(conn, "250 localhost\r\n")
			case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM:"):
				_, _ = fmt.Fprint(conn, "250 sender accepted\r\n")
			case strings.HasPrefix(strings.ToUpper(line), "RCPT TO:"):
				_, _ = fmt.Fprint(conn, "250 recipient accepted\r\n")
			case strings.EqualFold(strings.TrimSpace(line), "DATA"):
				_, _ = fmt.Fprint(conn, "354 send data\r\n")
				var message strings.Builder
				for {
					dataLine, readErr := reader.ReadString('\n')
					if readErr != nil {
						return
					}
					if dataLine == ".\r\n" {
						break
					}
					if strings.HasPrefix(dataLine, "..") {
						dataLine = dataLine[1:]
					}
					message.WriteString(dataLine)
				}
				received <- message.String()
				_, _ = fmt.Fprint(conn, "250 queued\r\n")
			case strings.EqualFold(strings.TrimSpace(line), "QUIT"):
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			}
		}
	}()
	return listener.Addr().String(), received
}

func parseAddress(value string) (mail.Address, error) {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return mail.Address{}, err
	}
	return *address, nil
}
