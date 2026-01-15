package main

import (
	"fmt"
	"sync/atomic"
)

var verbose bool

// requestCounter is used to generate incrementing request IDs
var requestCounter uint64

// nextRequestID returns the next incrementing request ID in UUID format and sequence number
func nextRequestID() (string, uint16) {
	id := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", id), uint16(id)
}

func debugf(format string, args ...any) {
	if verbose {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func isTextData(data []byte) bool {
	for _, b := range data {
		if b < 32 && b != 9 && b != 10 && b != 13 || b > 126 {
			return false
		}
	}
	return true
}

// printHexDump prints data in hex dump format
func printHexDump(data []byte) {
	for i := 0; i < len(data); i += 16 {
		// Address
		fmt.Printf("%04x  ", i)

		// Hex bytes
		for j := 0; j < 16; j++ {
			if i+j < len(data) {
				fmt.Printf("%02x ", data[i+j])
			} else {
				fmt.Print("   ")
			}
			if j == 7 {
				fmt.Print(" ")
			}
		}

		// ASCII
		fmt.Print(" |")
		for j := 0; j < 16 && i+j < len(data); j++ {
			b := data[i+j]
			if b >= 32 && b < 127 {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}
}
