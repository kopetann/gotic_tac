package id

import "github.com/google/uuid"

type UUID struct{}

func NewUUID() *UUID {
	return &UUID{}
}

func (UUID) NewID() string {
	return uuid.NewString()
}
