package conf

var DefaultServerConfig = ServerConfig{
	Network: Network{
		Host: "0.0.0.0",
		Port: 25565,
	},
	OnlineMode: false,
}

type ServerConfig struct {
	Network Network
	OnlineMode bool
}

type Network struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}
