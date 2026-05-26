package kitchen

import (
	"bytes"
	"fmt"
	"testing"

	"challenge/client"
)

// TestPrinterFormatPreview emits the console output for a small scripted
// scenario so a human can sanity-check the printer in `go test -v` output.
// Always passes; it exists to document the format.
func TestPrinterFormatPreview(t *testing.T) {
	var buf bytes.Buffer
	k := New(WithOutput(&buf), WithoutTimers())

	k.Place(client.Order{ID: "abc123de", Name: "Cheeseburger", Temp: "hot", Freshness: 60})
	k.Place(client.Order{ID: "def456gh", Name: "Iced Coffee", Temp: "cold", Freshness: 60})
	k.Place(client.Order{ID: "ghi789jk", Name: "Sourdough", Temp: "room", Freshness: 60})
	k.Pickup("abc123de")

	fmt.Print("\n--- printer sample ---\n")
	fmt.Print(buf.String())
	fmt.Print("--- end sample ---\n")
}
