package websocket

// Config is the public adapter-owned configuration surface for the reference
// WebSocket transport.
//
// It intentionally stays focused on transport concerns only. Durable storage
// selection and Postgres DSN wiring belong to the embedding application or
// reference CLI layer, not the adapter API itself.
type Config struct {
	Addr        string
	SharedToken string
}
