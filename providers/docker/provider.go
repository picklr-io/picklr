package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type Provider struct {
	pb.UnimplementedProviderServer
	client *client.Client
}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) ensureClient() error {
	if p.client != nil {
		return nil
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	p.client = cli
	return nil
}

func (p *Provider) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	if err := p.ensureClient(); err != nil {
		return &pb.ConfigureResponse{
			Diagnostics: []*pb.Diagnostic{
				{
					Severity: pb.Diagnostic_ERROR,
					Summary:  "Failed to create Docker client",
					Detail:   err.Error(),
				},
			},
		}, nil
	}
	return &pb.ConfigureResponse{}, nil
}

func (p *Provider) Plan(ctx context.Context, req *pb.PlanRequest) (*pb.PlanResponse, error) {
	if req.DesiredConfigJson == nil && req.PriorStateJson != nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_DELETE}, nil
	}

	if req.PriorStateJson == nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
	}

	switch req.Type {
	case "docker_container":
		var desired ContainerConfig
		if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
			return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
		}

		var prior ContainerState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}

		if desired.Image != prior.ImageName {
			return &pb.PlanResponse{
				Action:            pb.PlanResponse_REPLACE,
				ChangedAttributes: []string{"image"},
			}, nil
		}
		return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil

	case "docker_network":
		return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil
	case "docker_volume":
		return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil
	}

	return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil
}

func (p *Provider) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if err := p.ensureClient(); err != nil {
		return nil, err
	}

	switch req.Type {
	case "docker_container":
		return p.applyContainer(ctx, req)
	case "docker_network":
		return p.applyNetwork(ctx, req)
	case "docker_volume":
		return p.applyVolume(ctx, req)
	case "docker_image":
		return p.applyImage(ctx, req)
	}

	return nil, fmt.Errorf("unknown resource type: %s", req.Type)
}

func (p *Provider) applyImage(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// DELETE
	if req.DesiredConfigJson == nil {
		var prior ImageState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.client.ImageRemove(ctx, prior.ID, image.RemoveOptions{Force: true})
			if err != nil {
				if !client.IsErrNotFound(err) {
					return nil, fmt.Errorf("failed to remove image: %w", err)
				}
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ImageConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// BUILD
	if desired.BuildContext != "" {
		tar, err := archive.TarWithOptions(desired.BuildContext, &archive.TarOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create build context tar: %w", err)
		}

		opts := types.ImageBuildOptions{
			Tags:       []string{desired.Name},
			Dockerfile: desired.Dockerfile,
			Remove:     true,
		}

		resp, err := p.client.ImageBuild(ctx, tar, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to build image: %w", err)
		}
		defer resp.Body.Close()

		// Drain output to prevent blocking
		io.Copy(os.Stdout, resp.Body)
	} else {
		// PULL only
		reader, err := p.client.ImagePull(ctx, desired.Name, image.PullOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to pull image: %w", err)
		}
		io.Copy(os.Stdout, reader)
		reader.Close()
	}

	// Inspect to get ID
	inspect, _, err := p.client.ImageInspectWithRaw(ctx, desired.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect built image: %w", err)
	}

	newState := ImageState{
		ID:   inspect.ID,
		Name: desired.Name,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyContainer(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil { // Deletion
		var prior ContainerState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}

		if prior.ID != "" {
			timeout := 10 // seconds
			_ = p.client.ContainerStop(ctx, prior.ID, container.StopOptions{Timeout: &timeout})
			if err := p.client.ContainerRemove(ctx, prior.ID, container.RemoveOptions{Force: true}); err != nil {
				if !client.IsErrNotFound(err) {
					return nil, fmt.Errorf("failed to remove container: %w", err)
				}
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ContainerConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	reader, err := p.client.ImagePull(ctx, desired.Image, image.PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", desired.Image, err)
	}
	io.Copy(os.Stdout, reader)
	reader.Close()

	portBindings := nat.PortMap{}
	for hostPort, containerPort := range desired.Ports {
		p := nat.Port(fmt.Sprintf("%d/tcp", containerPort))
		portBindings[p] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: hostPort,
			},
		}
	}

	var binds []string
	for _, v := range desired.Volumes {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) > 0 {
			if strings.HasPrefix(parts[0], "./") || strings.HasPrefix(parts[0], "../") {
				abs, err := filepath.Abs(parts[0])
				if err == nil {
					parts[0] = abs
					binds = append(binds, strings.Join(parts, ":"))
					continue
				}
			}
		}
		binds = append(binds, v)
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        binds,
	}
	if len(desired.Networks) > 0 {
		hostConfig.NetworkMode = container.NetworkMode(desired.Networks[0])
	}

	if desired.Restart != "" {
		hostConfig.RestartPolicy = container.RestartPolicy{
			Name: container.RestartPolicyMode(desired.Restart),
		}
	}

	if desired.Logging != nil {
		hostConfig.LogConfig = container.LogConfig{
			Type:   desired.Logging.Driver,
			Config: desired.Logging.Options,
		}
	}

	// Mount secrets
	for _, secret := range desired.Secrets {
		absPath, err := filepath.Abs(secret.File)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve secret file path: %w", err)
		}
		hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s:ro", absPath, secret.Target))
	}

	config := &container.Config{
		Image:      desired.Image,
		Cmd:        desired.Command,
		Env:        mapToEnvList(desired.Env),
		Labels:     desired.Labels,
		WorkingDir: desired.WorkingDir,
		User:       desired.User,
	}

	if desired.Healthcheck != nil {
		test := desired.Healthcheck.Test
		if len(test) == 0 {
			test = []string{"NONE"}
		}

		interval, _ := time.ParseDuration(desired.Healthcheck.Interval)
		timeout, _ := time.ParseDuration(desired.Healthcheck.Timeout)
		startPeriod, _ := time.ParseDuration(desired.Healthcheck.StartPeriod)

		config.Healthcheck = &container.HealthConfig{
			Test:        test,
			Interval:    interval,
			Timeout:     timeout,
			StartPeriod: startPeriod,
			Retries:     desired.Healthcheck.Retries,
		}
	}

	resp, err := p.client.ContainerCreate(ctx,
		config,
		hostConfig,
		&network.NetworkingConfig{},
		&v1.Platform{},
		desired.Name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	if err := p.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	newState := ContainerState{
		ID:        resp.ID,
		Name:      desired.Name,
		ImageName: desired.Image,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyNetwork(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior NetworkState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			if err := p.client.NetworkRemove(ctx, prior.ID); err != nil {
				if !client.IsErrNotFound(err) {
					return nil, fmt.Errorf("failed to remove network: %w", err)
				}
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired NetworkConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	resp, err := p.client.NetworkCreate(ctx, desired.Name, types.NetworkCreate{
		Driver:   desired.Driver,
		Internal: desired.Internal,
		Labels:   desired.Labels,
		// CheckDuplicate removed in newer SDK
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}

	newState := NetworkState{
		ID:     resp.ID,
		Name:   desired.Name,
		Driver: desired.Driver,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyVolume(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VolumeState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			if err := p.client.VolumeRemove(ctx, prior.Name, true); err != nil {
				if !client.IsErrNotFound(err) {
					return nil, fmt.Errorf("failed to remove volume: %w", err)
				}
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VolumeConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	vol, err := p.client.VolumeCreate(ctx, volume.CreateOptions{
		Name:   desired.Name,
		Driver: desired.Driver,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}

	newState := VolumeState{
		Name:   vol.Name,
		Driver: vol.Driver,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func mapToEnvList(m map[string]string) []string {
	var env []string
	for k, v := range m {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

type ContainerConfig struct {
	Image       string             `json:"image"`
	Name        string             `json:"name"`
	Command     []string           `json:"command"`
	Ports       map[string]int     `json:"ports"`
	Env         map[string]string  `json:"env"`
	Networks    []string           `json:"networks"`
	Volumes     []string           `json:"volumes"`
	Labels      map[string]string  `json:"labels"`
	WorkingDir  string             `json:"workingDir"`
	User        string             `json:"user"`
	Restart     string             `json:"restart"`
	Healthcheck *HealthcheckConfig `json:"healthcheck"`
	Logging     *LoggingConfig     `json:"logging"`
	Secrets     []SecretConfig     `json:"secrets"`
}

type HealthcheckConfig struct {
	Test        []string `json:"test"`
	Interval    string   `json:"interval"`
	Timeout     string   `json:"timeout"`
	StartPeriod string   `json:"startPeriod"`
	Retries     int      `json:"retries"`
}

type LoggingConfig struct {
	Driver  string            `json:"driver"`
	Options map[string]string `json:"options"`
}

type SecretConfig struct {
	Source string `json:"source"`
	Target string `json:"target"`
	File   string `json:"file"`
}

type ContainerState struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ImageName string `json:"image"`
}

type NetworkConfig struct {
	Name           string            `json:"name"`
	Driver         string            `json:"driver"`
	CheckDuplicate bool              `json:"checkDuplicate"`
	Internal       bool              `json:"internal"`
	Labels         map[string]string `json:"labels"`
}

type NetworkState struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

type VolumeConfig struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

type VolumeState struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

type ImageConfig struct {
	Name         string `json:"name"`
	BuildContext string `json:"buildContext"`
	Dockerfile   string `json:"dockerfile"`
	Force        bool   `json:"force"`
}

type ImageState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
