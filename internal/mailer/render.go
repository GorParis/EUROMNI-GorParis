package mailer

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"math/big"
	"strings"
	texttemplate "text/template"

	"euromni/internal/store"
)

type Message struct {
	To          string
	Subject     string
	TextBody    string
	HTMLBody    string
	Idempotency string
}

type orderTemplateData struct {
	Language      string
	Title         string
	Greeting      string
	Intro         string
	OrderLabel    string
	ProductLabel  string
	QuantityLabel string
	TotalLabel    string
	SubtotalLabel string
	ShippingLabel string
	GrandLabel    string
	Thanks        string
	OrderNumber   string
	CustomerName  string
	Subtotal      string
	ShippingTotal string
	GrandTotal    string
	Items         []orderTemplateItem
}

type orderTemplateItem struct {
	Name      string
	Quantity  int
	LineTotal string
}

var orderHTMLTemplate = template.Must(template.New("order-confirmation").Parse(`<!doctype html>
<html lang="{{.Language}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{{.Title}}</title></head><body style="margin:0;background:#f4f5f7;font-family:Arial,sans-serif;color:#18212f">
<main style="max-width:640px;margin:24px auto;padding:28px;background:#fff;border-radius:12px">
<h1 style="font-size:24px">{{.Title}}</h1><p>{{.Greeting}}</p><p>{{.Intro}}</p>
<p><strong>{{.OrderLabel}}:</strong> {{.OrderNumber}}</p>
<table style="width:100%;border-collapse:collapse"><thead><tr><th align="left">{{.ProductLabel}}</th><th align="right">{{.QuantityLabel}}</th><th align="right">{{.TotalLabel}}</th></tr></thead>
<tbody>{{range .Items}}<tr><td style="padding:8px 0;border-top:1px solid #ddd">{{.Name}}</td><td align="right">{{.Quantity}}</td><td align="right">{{.LineTotal}}</td></tr>{{end}}</tbody></table>
<p>{{.SubtotalLabel}}: {{.Subtotal}}<br>{{.ShippingLabel}}: {{.ShippingTotal}}<br><strong>{{.GrandLabel}}: {{.GrandTotal}}</strong></p>
<p>{{.Thanks}}</p></main></body></html>`))

var orderTextTemplate = texttemplate.Must(texttemplate.New("order-confirmation-text").Parse(`{{.Title}}

{{.Greeting}}
{{.Intro}}
{{.OrderLabel}}: {{.OrderNumber}}

{{.ProductLabel}} — {{.QuantityLabel}} — {{.TotalLabel}}
{{range .Items}}{{.Name}} — {{.Quantity}} — {{.LineTotal}}
{{end}}
{{.SubtotalLabel}}: {{.Subtotal}}
{{.ShippingLabel}}: {{.ShippingTotal}}
{{.GrandLabel}}: {{.GrandTotal}}

{{.Thanks}}
`))

func RenderOrderConfirmation(payload store.OrderConfirmationPayload, recipient, idempotency string) (Message, error) {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" || payload.OrderNumber == "" || payload.Currency == "" || len(payload.Items) == 0 {
		return Message{}, errors.New("incomplete order confirmation")
	}
	if _, ok := new(big.Rat).SetString(payload.Subtotal); !ok {
		return Message{}, errors.New("invalid order subtotal")
	}
	if _, ok := new(big.Rat).SetString(payload.ShippingTotal); !ok {
		return Message{}, errors.New("invalid order shipping total")
	}
	if _, ok := new(big.Rat).SetString(payload.GrandTotal); !ok {
		return Message{}, errors.New("invalid order grand total")
	}
	french := strings.HasPrefix(strings.ToLower(payload.Locale), "fr")
	data := orderTemplateData{
		Language: "en", Title: "Order confirmation", Greeting: "Hello " + payload.CustomerName + ",",
		Intro:      "We have received your order and it is awaiting payment.",
		OrderLabel: "Order", ProductLabel: "Product", QuantityLabel: "Quantity",
		TotalLabel: "Total", SubtotalLabel: "Subtotal", ShippingLabel: "Shipping",
		GrandLabel: "Grand total", Thanks: "Thank you for your order.",
		OrderNumber:   payload.OrderNumber,
		CustomerName:  payload.CustomerName,
		Subtotal:      formatAmount(payload.Subtotal, payload.Currency),
		ShippingTotal: formatAmount(payload.ShippingTotal, payload.Currency),
		GrandTotal:    formatAmount(payload.GrandTotal, payload.Currency),
		Items:         make([]orderTemplateItem, len(payload.Items)),
	}
	if french {
		data.Language = "fr"
		data.Title = "Confirmation de commande"
		data.Greeting = "Bonjour " + payload.CustomerName + ","
		data.Intro = "Nous avons bien reçu votre commande. Son paiement est en attente."
		data.OrderLabel = "Commande"
		data.ProductLabel = "Produit"
		data.QuantityLabel = "Quantité"
		data.TotalLabel = "Total"
		data.SubtotalLabel = "Sous-total"
		data.ShippingLabel = "Livraison"
		data.GrandLabel = "Total de la commande"
		data.Thanks = "Merci pour votre commande."
	}
	for index, item := range payload.Items {
		if item.Quantity < 1 {
			return Message{}, errors.New("invalid order item quantity")
		}
		if _, ok := new(big.Rat).SetString(item.LineTotal); !ok {
			return Message{}, errors.New("invalid order item total")
		}
		data.Items[index] = orderTemplateItem{
			Name: item.Name, Quantity: item.Quantity,
			LineTotal: formatAmount(item.LineTotal, payload.Currency),
		}
	}
	var htmlBody, textBody bytes.Buffer
	if err := orderHTMLTemplate.Execute(&htmlBody, data); err != nil {
		return Message{}, fmt.Errorf("render order confirmation HTML: %w", err)
	}
	if err := orderTextTemplate.Execute(&textBody, data); err != nil {
		return Message{}, fmt.Errorf("render order confirmation text: %w", err)
	}
	subject := "Order confirmation " + payload.OrderNumber
	if french {
		subject = "Confirmation de commande " + payload.OrderNumber
	}
	return Message{
		To: recipient, Subject: subject, TextBody: textBody.String(),
		HTMLBody: htmlBody.String(), Idempotency: idempotency,
	}, nil
}

func formatAmount(amount, currency string) string {
	rational, _ := new(big.Rat).SetString(amount)
	return rational.FloatString(2) + " " + currency
}
