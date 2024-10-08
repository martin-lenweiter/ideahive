package testutil

import (
	"fmt"
	"ideahive/backend/cmd/app"
	"ideahive/backend/config"
	"ideahive/backend/internal/database"
	"log"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ideahive/backend/internal/handlers"
	"ideahive/backend/internal/models"
	"ideahive/backend/internal/services"
)

var (
	TestDB     *gorm.DB
	pool       *dockertest.Pool
	resource   *dockertest.Resource
	TestServer *httptest.Server
	TestApp    *app.App
)

func NewTestApp(cfg *config.Config, db *database.Database) (*app.App, error) {
	// Initialize services
	svc := services.New(db)

	// Initialize handlers
	h := handlers.New(svc)

	// Set up router
	router := chi.NewRouter()

	testApp := &app.App{
		Config:   cfg,
		DB:       db.GormDB,
		Router:   router,
		Handlers: h,
	}

	testApp.Routes()

	return testApp, nil
}

func SetupTestEnvironment() error {
	var err error

	// Setup test database
	TestDB, err = SetupTestDB()
	if err != nil {
		return err
	}

	// Create a mock config
	mockConfig := &config.Config{
		ServerAddress: ":8080",
		DatabaseURL:   GetTestDatabaseURL(),
	}

	// Create a mock database instance
	dbInstance := &database.Database{GormDB: TestDB}

	// Initialize the App with test configurations
	TestApp, err = NewTestApp(mockConfig, dbInstance)
	if err != nil {
		return err
	}

	// Create test server
	TestServer = httptest.NewServer(TestApp.Router)

	return nil
}

func TeardownTestEnvironment() {
	if TestServer != nil {
		TestServer.Close()
	}
	TeardownTestDB()
}

func GetTestDatabaseURL() string {
	return fmt.Sprintf("postgres://postgres:secret@localhost:%s/testdb?sslmode=disable", resource.GetPort("5432/tcp"))
}

func SetupTestDB() (*gorm.DB, error) {
	var err error
	pool, err = dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	resource, err = pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "14",
		Env: []string{
			// todo: move to env file
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_DB=testdb",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	if err != nil {
		log.Fatalf("Could not start resource: %s", err)
	}

	databaseURL := GetTestDatabaseURL()

	if err := pool.Retry(func() error {
		var err error
		TestDB, err = gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
		if err != nil {
			return err
		}
		sqlDB, err := TestDB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Ping()
	}); err != nil {
		log.Fatalf("Could not connect to database after multiple attempts: %s", err)
	}

	// Run migrations
	err = TestDB.AutoMigrate(&models.Idea{}) // add other models as needed
	if err != nil {
		log.Fatalf("Could not run migrations: %s", err)
	}

	return TestDB, nil
}

func TeardownTestDB() {
	if err := pool.Purge(resource); err != nil {
		log.Fatalf("Could not purge resource: %s", err)
	}
}
