package service

import "errors"

// Custom errors for giveaway service
var (
	ErrNotFound = errors.New("giveaway not found")
	ErrNotOwner = errors.New("you are not the owner of this giveaway")
)
