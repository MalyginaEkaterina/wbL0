package app

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
	"wbL0/internal/publisher"
	"wbL0/internal/publisher/app"
)

func TestApp(t *testing.T) {
	ctx := context.Background()

	netw, err := network.New(ctx)
	require.NoError(t, err)

	pgContainer, err := NewPostgreSQLContainer(ctx, netw)
	defer pgContainer.Terminate(ctx)
	require.NoError(t, err)

	stanContainer, err := NewStanContainer(ctx, netw)
	defer stanContainer.Terminate(ctx)
	require.NoError(t, err)

	backContainer, err := NewBackendContainer(ctx, netw)
	defer backContainer.Terminate(ctx)
	require.NoError(t, err)

	natsURL, err := stanContainer.getURL()
	require.NoError(t, err)
	publishConfig := publisher.Config{
		NatsURL:       natsURL,
		StanClusterID: "test-cluster",
		ClientID:      "testpub",
		ChannelName:   "testchannel",
	}

	id := uuid.NewString()

	//Проверяем, что вернет ошибку Not Found в случае, когда нет заказа с данным id
	url, err := backContainer.getURL()
	require.NoError(t, err)
	endpoint := fmt.Sprintf("%s/api/?id=%s", url, id)
	response, err := http.Get(endpoint)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)

	//Публикуем заказ с данным id в NATS streaming
	msg, err := app.GenerateMsg("msg.json", id)
	require.NoError(t, err)
	err = app.Publish(publishConfig, msg)
	require.NoError(t, err)

	//Ожидаем, пока сообщение будет вычитано из очереди и обработано приложением
	time.Sleep(30 * time.Second)

	//Проверяем, что по данному id теперь возвращается то, что было отправлено в NATS streaming
	response, err = http.Get(endpoint)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, body, msg)

	//Перезапускаем приложение
	err = backContainer.Stop(ctx, nil)
	require.NoError(t, err)

	err = backContainer.Start(ctx)
	require.NoError(t, err)

	//Проверяем, что по данному id возвращаются корректные данные после перезапуска приложения
	url, err = backContainer.getURL()
	require.NoError(t, err)
	endpoint = fmt.Sprintf("%s/api/?id=%s", url, id)
	response, err = http.Get(endpoint)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	body, err = io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, body, msg)
}

type BackendContainer struct {
	testcontainers.Container
}

func (c *BackendContainer) getURL() (string, error) {
	host, err := c.Host(context.Background())
	if err != nil {
		return "", err
	}
	port, err := c.MappedPort(context.Background(), "8080")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), nil
}

func NewBackendContainer(ctx context.Context, netw *testcontainers.DockerNetwork) (*BackendContainer, error) {
	os.Chdir("../../../")
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    ".",
			Dockerfile: "Dockerfile",
		},
		Env: map[string]string{
			"ADDRESS":         ":8080",
			"NATS_URL":        "nats://nats:4222",
			"BACK_CLIENT_ID":  "testback",
			"CHANNEL_NAME":    "testchannel",
			"DATABASE_URI":    "user=postgres password=12345 host=pgsql port=5432 dbname=postgres sslmode=disable",
			"DURABLE_NAME":    "testdurable",
			"STAN_CLUSTER_ID": "test-cluster",
		},
		ExposedPorts: []string{"8080/tcp"},
		Networks:     []string{netw.Name},
		WaitingFor:   wait.ForAll(wait.ForListeningPort("8080/tcp")),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	return &BackendContainer{
		Container: container,
	}, nil
}

type StanContainer struct {
	testcontainers.Container
}

func (c *StanContainer) getURL() (string, error) {
	host, err := c.Host(context.Background())
	if err != nil {
		return "", err
	}
	port, err := c.MappedPort(context.Background(), "4222")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("nats://%s:%s", host, port.Port()), nil
}

func NewStanContainer(ctx context.Context, netw *testcontainers.DockerNetwork) (*StanContainer, error) {
	req := testcontainers.ContainerRequest{
		Env: map[string]string{
			"STAN_STORE": "MEMORY",
		},
		ExposedPorts:   []string{"4222/tcp"},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"nats"}},
		Image:          "nats-streaming:latest",
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	return &StanContainer{
		Container: container,
	}, nil
}

func NewPostgreSQLContainer(ctx context.Context, netw *testcontainers.DockerNetwork) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "12345",
			"POSTGRES_DB":       "postgres",
		},
		Networks:       []string{netw.Name},
		NetworkAliases: map[string][]string{netw.Name: {"pgsql"}},
		Image:          "postgres:latest",
		WaitingFor: wait.ForExec([]string{"pg_isready"}).
			WithPollInterval(1 * time.Second).
			WithExitCodeMatcher(func(exitCode int) bool {
				return exitCode == 0
			}),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	return container, nil
}
