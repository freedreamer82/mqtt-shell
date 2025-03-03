package mqttcp

type MqttJsonCp struct {
	UUID       string            `json:"uuid"`
	Step       string            `json:"step"`
	ClientUUID string            `json:"clientuuid"`
	Request    MqttJsonCpRequest `json:"request"`
	Ts         int64             `json:"ts"`
	Error      string            `json:"error"`
	Topic      string            `json:"topic"`
	EndStr     string            `json:"endStr"`
}

type MqttJsonCpRequest struct {
	Cmd        string `json:"cmd"`
	ClientPath string `json:"clientpath"`
	ServerPath string `json:"serverpath"`
	Size       int64  `json:"size"`
	MD5        string `json:"md5"`
	Protocol   string `json:"protocol"`
}
