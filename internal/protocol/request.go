package protocol

import (
	"fmt"
	"sync/atomic"
)

// requestCounter is used to generate incrementing request IDs
var requestCounter uint64

// NextRequestID returns the next incrementing request ID in UUID format and sequence number
func NextRequestID() (string, uint16) {
	id := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", id), uint16(id)
}
