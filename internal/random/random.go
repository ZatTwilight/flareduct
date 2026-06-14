package random

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func HexSuffix(byteLen int) string {
	if byteLen <= 0 {
		byteLen = 3
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())[:byteLen*2]
	}
	return hex.EncodeToString(buf)
}
