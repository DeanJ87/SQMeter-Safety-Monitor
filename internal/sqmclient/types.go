package sqmclient

// StatusResponse is the payload from GET /api/status.
type StatusResponse struct {
	Uptime   int    `json:"uptime"`
	FreeHeap int    `json:"freeHeap"`
	RSSI     int    `json:"rssi"`
	IP       string `json:"ip"`
	Version  string `json:"version"`
}

// SensorsResponse is the payload from GET /api/sensors.
type SensorsResponse struct {
	LightSensor     LightSensor     `json:"lightSensor"`
	SkyQuality      SkyQuality      `json:"skyQuality"`
	Environment     Environment     `json:"environment"`
	IRTemperature   IRTemperature   `json:"irTemperature"`
	CloudConditions CloudConditions `json:"cloudConditions"`
	GPS             *GPS            `json:"gps,omitempty"`
}

// Sensor status values: 0=OK, 1=Not found, 2=Read error, 3=Stale data.

// LightSensor holds readings from the visible/IR light sensor.
type LightSensor struct {
	Lux      float64 `json:"lux"`
	Visible  int     `json:"visible"`
	Infrared int     `json:"infrared"`
	Full     int     `json:"full"`
	Status   int     `json:"status"`
}

// SkyQuality holds sky brightness readings.
type SkyQuality struct {
	SQM         float64 `json:"sqm"`
	NELM        float64 `json:"nelm"`
	Bortle      float64 `json:"bortle"`
	Description string  `json:"description"`
}

// Environment holds temperature/humidity readings.
type Environment struct {
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	Pressure    float64 `json:"pressure"`
	Dewpoint    float64 `json:"dewpoint"`
	Status      int     `json:"status"`
}

// IRTemperature holds readings from the MLX90614 IR thermometer.
type IRTemperature struct {
	ObjectTemp  float64 `json:"objectTemp"`
	AmbientTemp float64 `json:"ambientTemp"`
	Status      int     `json:"status"`
}

// CloudConditions holds cloud-cover estimates derived from the IR sensor.
type CloudConditions struct {
	TemperatureDelta  float64 `json:"temperatureDelta"`
	CorrectedDelta    float64 `json:"correctedDelta"`
	CloudCoverPercent float64 `json:"cloudCoverPercent"`
	Condition         int     `json:"condition"`
	Description       string  `json:"description"`
	HumidityUsed      float64 `json:"humidityUsed"`
}

// GPS holds optional GPS data, present only when a GPS module is attached.
type GPS struct {
	HasFix     bool    `json:"hasFix"`
	Satellites int     `json:"satellites"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Altitude   float64 `json:"altitude"`
	HDOP       float64 `json:"hdop"`
	Age        int     `json:"age"`
}
