// Run once per installation. Keep the private key in backend secret storage.
package main

import (
	"fmt"
	webpush "github.com/SherClockHolmes/webpush-go"
	"os"
)

func main() {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "key generation failed")
		os.Exit(1)
	}
	fmt.Printf("WEB_PUSH_VAPID_PUBLIC_KEY=%s\nWEB_PUSH_VAPID_PRIVATE_KEY=%s\n", public, private)
}
