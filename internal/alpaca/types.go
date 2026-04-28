package alpaca

// ASCOM Alpaca error numbers.
const (
	ErrOK             = 0
	ErrNotConnected   = 0x0400 // 1024
	ErrInvalidValue   = 0x0401 // 1025
	ErrNotImplemented = 0x040B // 1035
	ErrInvalidOp      = 0x040C // 1036
	ErrUnspecified    = 0x04FF // 1279
)

// Response is the standard Alpaca envelope for endpoints that return a value.
type Response[T any] struct {
	Value               T      `json:"Value"`
	ClientTransactionID uint32 `json:"ClientTransactionID"`
	ServerTransactionID uint32 `json:"ServerTransactionID"`
	ErrorNumber         int    `json:"ErrorNumber"`
	ErrorMessage        string `json:"ErrorMessage"`
}

// VoidResponse is the standard Alpaca envelope for void endpoints (PUTs
// that return no value).
type VoidResponse struct {
	ClientTransactionID uint32 `json:"ClientTransactionID"`
	ServerTransactionID uint32 `json:"ServerTransactionID"`
	ErrorNumber         int    `json:"ErrorNumber"`
	ErrorMessage        string `json:"ErrorMessage"`
}

// ConfiguredDevice describes a device exposed by this Alpaca server.
type ConfiguredDevice struct {
	DeviceName   string `json:"DeviceName"`
	DeviceType   string `json:"DeviceType"`
	DeviceNumber int    `json:"DeviceNumber"`
	UniqueID     string `json:"UniqueID"`
}

// ServerDescription describes this Alpaca server instance.
type ServerDescription struct {
	ServerName          string `json:"ServerName"`
	Manufacturer        string `json:"Manufacturer"`
	ManufacturerVersion string `json:"ManufacturerVersion"`
	Location            string `json:"Location"`
}

// DeviceStateItem is a single property returned by the devicestate endpoint.
type DeviceStateItem struct {
	Name  string `json:"Name"`
	Value any    `json:"Value"`
}
