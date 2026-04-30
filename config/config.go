package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App       AppConfig
	JWT       JWTConfig
	Database  DatabaseConfig
	Supabase  SupabaseConfig
	Redis     RedisConfig
	RabbitMQ  RabbitMQConfig
	Gemini    GeminiConfig
	Worker    WorkerConfig
	Upload    UploadConfig
	RateLimit RateLimitConfig
}

type AppConfig struct {
	Env     string
	Port    string
	BaseURL string
}

type JWTConfig struct {
	Secret              string
	AccessExpireMinutes int
	RefreshExpireDays   int
}

type DatabaseConfig struct {
	URL            string
	MaxConnections int32
	MinConnections int32
}

type SupabaseConfig struct {
	URL        string
	AnonKey    string
	ServiceKey string
	Bucket     string
}

type RedisConfig struct {
	URL string
}

type RabbitMQConfig struct {
	URL           string
	QueueName     string
	PrefetchCount int
}

type GeminiConfig struct {
	APIKey         string
	Model          string
	EmbeddingModel string
	MaxRetries     int
}

type WorkerConfig struct {
	PoolSize int
	RetryMax int
}

type UploadConfig struct {
	MaxFileSizeMB      int64
	AllowedFileTypes   []string
	PresignedURLExpire time.Duration
}

type RateLimitConfig struct {
	UploadRPS  float64
	GeneralRPS float64
}

// Load reads configuration from environment variables.
// It expects a .env file in development or env vars set in production.
func Load() (*Config, error) {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	// Read .env file if it exists; ignore error in production
	// where env vars are injected directly
	_ = viper.ReadInConfig()

	cfg := &Config{
		App: AppConfig{
			Env:     viper.GetString("APP_ENV"),
			Port:    viper.GetString("APP_PORT"),
			BaseURL: viper.GetString("APP_BASE_URL"),
		},
		JWT: JWTConfig{
			Secret:              viper.GetString("JWT_SECRET"),
			AccessExpireMinutes: viper.GetInt("JWT_ACCESS_EXPIRE_MINUTES"),
			RefreshExpireDays:   viper.GetInt("JWT_REFRESH_EXPIRE_DAYS"),
		},
		Database: DatabaseConfig{
			URL:            viper.GetString("DATABASE_URL"),
			MaxConnections: int32(viper.GetInt("DATABASE_MAX_CONNECTIONS")),
			MinConnections: int32(viper.GetInt("DATABASE_MIN_CONNECTIONS")),
		},
		Supabase: SupabaseConfig{
			URL:        viper.GetString("SUPABASE_URL"),
			AnonKey:    viper.GetString("SUPABASE_ANON_KEY"),
			ServiceKey: viper.GetString("SUPABASE_SERVICE_KEY"),
			Bucket:     viper.GetString("SUPABASE_BUCKET"),
		},
		Redis: RedisConfig{
			URL: viper.GetString("REDIS_URL"),
		},
		RabbitMQ: RabbitMQConfig{
			URL:           viper.GetString("RABBITMQ_URL"),
			QueueName:     viper.GetString("RABBITMQ_QUEUE_NAME"),
			PrefetchCount: viper.GetInt("RABBITMQ_PREFETCH_COUNT"),
		},
		Gemini: GeminiConfig{
			APIKey:         viper.GetString("GEMINI_API_KEY"),
			Model:          viper.GetString("GEMINI_MODEL"),
			EmbeddingModel: viper.GetString("GEMINI_EMBEDDING_MODEL"),
			MaxRetries:     viper.GetInt("GEMINI_MAX_RETRIES"),
		},
		Worker: WorkerConfig{
			PoolSize: viper.GetInt("WORKER_POOL_SIZE"),
			RetryMax: viper.GetInt("WORKER_RETRY_MAX"),
		},
		Upload: UploadConfig{
			MaxFileSizeMB:      viper.GetInt64("MAX_FILE_SIZE_MB"),
			AllowedFileTypes:   strings.Split(viper.GetString("ALLOWED_FILE_TYPES"), ","),
			PresignedURLExpire: time.Duration(viper.GetInt("PRESIGNED_URL_EXPIRE_MINUTES")) * time.Minute,
		},
		RateLimit: RateLimitConfig{
			UploadRPS:  viper.GetFloat64("RATE_LIMIT_UPLOAD_RPS"),
			GeneralRPS: viper.GetFloat64("RATE_LIMIT_GENERAL_RPS"),
		},
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required config values are present.
func validate(cfg *Config) error {
	required := map[string]string{
		"JWT_SECRET":           cfg.JWT.Secret,
		"DATABASE_URL":         cfg.Database.URL,
		"SUPABASE_URL":         cfg.Supabase.URL,
		"SUPABASE_SERVICE_KEY": cfg.Supabase.ServiceKey,
		"SUPABASE_BUCKET":      cfg.Supabase.Bucket,
		"REDIS_URL":            cfg.Redis.URL,
		"RABBITMQ_URL":         cfg.RabbitMQ.URL,
		"GEMINI_API_KEY":       cfg.Gemini.APIKey,
	}

	for key, val := range required {
		if val == "" {
			return &MissingConfigError{Key: key}
		}
	}

	return nil
}

// MissingConfigError is returned when a required config key is not set.
type MissingConfigError struct {
	Key string
}

func (e *MissingConfigError) Error() string {
	return "missing required config: " + e.Key
}

// IsDevelopment returns true when running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

// IsProduction returns true when running in production mode.
func (c *Config) IsProduction() bool {
	return c.App.Env == "production"
}
