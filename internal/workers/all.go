package workers

// Every package that declares job classes, so a binary importing workers
// can perform all of them.
import (
	_ "github.com/dodopok/estevao-api-go/internal/activestorage"
	_ "github.com/dodopok/estevao-api-go/internal/audio"
	_ "github.com/dodopok/estevao-api-go/internal/audiogen"
	_ "github.com/dodopok/estevao-api-go/internal/notify"
	_ "github.com/dodopok/estevao-api-go/internal/rosary"
)
