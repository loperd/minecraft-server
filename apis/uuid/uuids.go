package uuid

import (
	"encoding/binary"
	"fmt"
	"github.com/satori/go.uuid"
	"strings"
)

type UUID = uuid.UUID

func NewUUID() UUID {
	gen := uuid.NewV4()
	/*if err != nil {
		panic(err)
	}*/

	return gen
}

func FromString(text string) UUID {
	result, err := uuid.FromString(text)

	if err != nil {
		panic(fmt.Errorf("failed to decode uuid for %s: %v\n", text, err))
	}

	return result
}

func TextToUUID(text string) UUID {
	bytes := make([]byte, 16)

	_, err := strings.NewReader(text).Read(bytes)
	if err != nil {
		panic(fmt.Errorf("failed to create uuid for %s: %v\n", text, err))
	}

	result, err := uuid.FromBytes(bytes)
	if err != nil {
		panic(fmt.Errorf("failed to create uuid for %s: %v\n", text, err))
	}

	return result
}

func BitsToUUID(msb, lsb int64) (data UUID, err error) {
	mBytes := make([]byte, 8)
	lBytes := make([]byte, 8)

	binary.BigEndian.PutUint64(mBytes, uint64(msb))
	binary.BigEndian.PutUint64(lBytes, uint64(lsb))

	return uuid.FromBytes(append(mBytes, lBytes...))
}

func UUIDToText(uuid UUID) (text string, err error) {
	data, err := uuid.MarshalText()

	if err == nil {
		text = string(data)
	}

	return
}

func SigBits(uuid UUID) (msb, lsb int64) {
	bytes := uuid.Bytes()

	msb = 0
	lsb = 0

	for i := 0; i < 8; i++ {
		msb = (msb << 0x08) | int64(bytes[i]&0xFF)
	}

	for i := 8; i < 16; i++ {
		lsb = (lsb << 0x08) | int64(bytes[i]&0xFF)
	}

	return
}
