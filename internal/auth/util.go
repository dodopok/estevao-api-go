package auth

import (
	"strconv"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func appZone() *time.Location { return rb.AppZone }
