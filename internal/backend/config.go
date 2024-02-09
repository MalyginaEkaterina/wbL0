package backend

type Config struct {
	Address       string `env:"ADDRESS"`
	DatabaseURI   string `env:"DATABASE_URI"`
	NatsURL       string `env:"NATS_URL"`
	StanClusterID string `env:"STAN_CLUSTER_ID"`
	ClientID      string `env:"BACK_CLIENT_ID"`
	ChannelName   string `env:"CHANNEL_NAME"`
	DurableName   string `env:"DURABLE_NAME"`
}
