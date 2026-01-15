package util

import "fmt"

// IsTextData checks if a byte slice contains only printable ASCII text
func IsTextData(data []byte) bool {
	for _, b := range data {
		if b < 32 && b != 9 && b != 10 && b != 13 || b > 126 {
			return false
		}
	}
	return true
}

// PrintHexDump prints data in hex dump format
func PrintHexDump(data []byte) {
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
