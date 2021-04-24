package ents

import (
	"github.com/golangmc/minecraft-server/apis/base"
)

type Sender interface {
	base.Named
	base.Unique

	SendMessage(message ...interface{})
}
