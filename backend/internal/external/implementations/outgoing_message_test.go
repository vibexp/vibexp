package implementations

import (
	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/external"
)

// outgoing wraps a gomail message with a display name, the shape every provider
// now receives. Shared by the four provider suites.
func outgoing(message *gomail.EmailMessage, fromName string) *external.OutgoingMessage {
	return &external.OutgoingMessage{Message: message, FromName: fromName}
}

// testSenderAddress is the From address every provider suite sends from.
const testSenderAddress = "hello@acme.test"

// testMessage builds a minimal message from testSenderAddress, for the suites
// that only care about what happens to the From value.
func testMessage() *gomail.EmailMessage {
	return gomail.NewFullEmailMessage(
		testSenderAddress,
		[]string{"recipient@example.test"},
		"Subject",
		nil, nil, "",
		"text body",
		"<p>html body</p>",
		nil,
	)
}
