package ents

type Entity interface {
	Sender

	EntityUUID() int64
}
