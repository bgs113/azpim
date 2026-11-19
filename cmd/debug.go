package cmd

import (
	"fmt"
	"io"
	"sync"
	"time"

	azlog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// debug is the --debug flag: log what the Azure SDK does to stderr.
var debug bool

// enableDebugLog writes the Azure SDK's log of sign-in steps, HTTP requests,
// responses and retries to w, each line stamped with the seconds since it was
// enabled, so a slow or retried call shows where the time went. The SDK
// redacts the Authorization header, other headers it doesn't know, and query
// parameter values, and never logs bodies.
func enableDebugLog(w io.Writer) {
	start := time.Now()
	var mu sync.Mutex // the SDK logs from concurrent requests
	azlog.SetListener(func(ev azlog.Event, msg string) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, "[debug %7.3fs] %s: %s\n", time.Since(start).Seconds(), ev, msg)
	})
	azlog.SetEvents(azlog.EventRequest, azlog.EventResponse, azlog.EventResponseError,
		azlog.EventRetryPolicy, azidentity.EventAuthentication)
}
