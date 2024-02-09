package publisher

type Config struct {
	NatsURL       string `env:"NATS_URL"`
	StanClusterID string `env:"STAN_CLUSTER_ID"`
	ClientID      string `env:"CLIENT_ID"`
	ChannelName   string `env:"CHANNEL_NAME"`
}
