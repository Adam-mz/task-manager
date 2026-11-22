package task

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"task-manager/internal/auth"
)

func serveAuthenticatedTaskRequest(t *testing.T, handler http.Handler, w http.ResponseWriter, r *http.Request, userID int64) {
	t.Helper()
	tokens, err := auth.NewTokenManager([]byte("0123456789abcdef0123456789abcdef"), time.Hour)
	if err != nil {
		t.Fatalf("Create token manager: %v", err)
	}
	token, err := tokens.Generate(userID)
	if err != nil {
		t.Fatalf("Generate token: %v", err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	tokens.Middleware(handler).ServeHTTP(w, r)
}

type timeoutRepository struct {
	Repository
}

func (r *timeoutRepository) GetByID(ctx context.Context, userID, id int64) (Task, error) {
	<-ctx.Done()
	return Task{}, ctx.Err()
}

func (r *timeoutRepository) Create(ctx context.Context, task Task) (Task, error) {
	<-ctx.Done()
	return Task{}, ctx.Err()
}

func (r *timeoutRepository) List(ctx context.Context, userID int64, options ListOptions) ([]Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *timeoutRepository) Update(ctx context.Context, task Task) (Task, error) {
	<-ctx.Done()
	return Task{}, ctx.Err()
}

func (r *timeoutRepository) Delete(ctx context.Context, userID, id int64) error {
	<-ctx.Done()
	return ctx.Err()
}

type errorRepository struct {
	Repository
	err error
}

func (r *errorRepository) Create(ctx context.Context, task Task) (Task, error) {
	return Task{}, r.err
}

func (r *errorRepository) List(ctx context.Context, userID int64, options ListOptions) ([]Task, error) {
	return nil, r.err
}

func (r *errorRepository) Update(ctx context.Context, task Task) (Task, error) {
	return Task{}, r.err
}

func (r *errorRepository) Delete(ctx context.Context, userID, id int64) error {
	return r.err
}

func TestHTTPHandlerCreateSuccess(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository)
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(
		http.MethodPost,
		"/tasks",
		strings.NewReader(`{"title":"Learn Go","description":"Study HTTP handlers"}`),
	)
	recorder := httptest.NewRecorder()

	serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.Create), recorder, request, 42)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("Create() status code = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Create() Content-Type = %q, want %q", contentType, "application/json")
	}

	var response taskResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response body: %v", err)
	}
	if response.ID != 1 {
		t.Errorf("Create() ID = %d, want 1", response.ID)
	}
	if response.Title != "Learn Go" {
		t.Errorf("Create() Title = %q, want %q", response.Title, "Learn Go")
	}
	if response.Description != "Study HTTP handlers" {
		t.Errorf("Create() Description = %q, want %q", response.Description, "Study HTTP handlers")
	}
	if response.Status != StatusTodo {
		t.Errorf("Create() Status = %q, want %q", response.Status, StatusTodo)
	}
	if response.CreatedAt.IsZero() {
		t.Error("Create() CreatedAt is zero")
	}
	stored, err := repository.GetByID(request.Context(), 42, response.ID)
	if err != nil {
		t.Fatalf("Get created task: %v", err)
	}
	if stored.UserID != 42 {
		t.Errorf("Created task UserID = %d, want authenticated user 42", stored.UserID)
	}
}

func TestHTTPHandlerCreateUnauthorized(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "valid JSON", body: `{"title":"Task","description":"Test"}`},
		{name: "broken JSON", body: `{"title":`},
		{name: "empty body", body: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := NewMemoryRepository()
			handler := NewHTTPHandler(NewService(repository))
			request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			handler.Create(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("Create() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response: %v", err)
			}
			if response.Error != "unauthorized" {
				t.Errorf("Error = %q, want unauthorized", response.Error)
			}
			tasks, err := repository.List(t.Context(), 42, ListOptions{Limit: 100})
			if err != nil {
				t.Fatalf("List tasks: %v", err)
			}
			if len(tasks) != 0 {
				t.Errorf("Unauthorized request created %d tasks, want 0", len(tasks))
			}
		})
	}
}

func TestHTTPHandlerCreateErrors(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "broken JSON",
			body:      `{"title":`,
			wantError: "invalid request body",
		},
		{
			name:      "unknown field",
			body:      `{"title":"Learn Go","description":"Test","priority":1}`,
			wantError: "invalid request body",
		},
		{
			name:      "empty title",
			body:      `{"title":"   ","description":"Test"}`,
			wantError: ErrTitleRequired.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := NewMemoryRepository()
			service := NewService(repository)
			handler := NewHTTPHandler(service)

			request := httptest.NewRequest(
				http.MethodPost,
				"/tasks",
				strings.NewReader(tt.body),
			)
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.Create), recorder, request, 42)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("Create() status code = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Create() Content-Type = %q, want %q", contentType, "application/json")
			}

			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response body: %v", err)
			}
			if response.Error != tt.wantError {
				t.Errorf("Create() error = %q, want %q", response.Error, tt.wantError)
			}
		})
	}
}

func TestHTTPHandlerList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		repository := NewMemoryRepository()
		service := NewService(repository)
		handler := NewHTTPHandler(service)

		request := httptest.NewRequest(http.MethodGet, "/tasks", nil)
		recorder := httptest.NewRecorder()

		serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.List), recorder, request, 42)

		if recorder.Code != http.StatusOK {
			t.Fatalf("List() status code = %d, want %d", recorder.Code, http.StatusOK)
		}
		if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
			t.Errorf("List() Content-Type = %q, want %q", contentType, "application/json")
		}

		var response []taskResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("Decode response body: %v", err)
		}
		if response == nil {
			t.Fatal("List() response is nil, want non-nil empty slice")
		}
		if len(response) != 0 {
			t.Errorf("List() response length = %d, want 0", len(response))
		}
	})

	t.Run("two tasks", func(t *testing.T) {
		repository := NewMemoryRepository()
		service := NewService(repository)
		handler := NewHTTPHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/tasks", nil)

		first, err := service.Create(request.Context(), 42, "First task", "First description")
		if err != nil {
			t.Fatalf("Create first task: %v", err)
		}
		second, err := service.Create(request.Context(), 42, "Second task", "Second description")
		if err != nil {
			t.Fatalf("Create second task: %v", err)
		}

		recorder := httptest.NewRecorder()
		serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.List), recorder, request, 42)

		if recorder.Code != http.StatusOK {
			t.Fatalf("List() status code = %d, want %d", recorder.Code, http.StatusOK)
		}

		var response []taskResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("Decode response body: %v", err)
		}
		if len(response) != 2 {
			t.Fatalf("List() response length = %d, want 2", len(response))
		}
		if response[0].ID != 1 || response[1].ID != 2 {
			t.Errorf("List() IDs = [%d, %d], want [1, 2]", response[0].ID, response[1].ID)
		}
		if response[0].Title != first.Title {
			t.Errorf("List() first title = %q, want %q", response[0].Title, first.Title)
		}
		if response[1].Title != second.Title {
			t.Errorf("List() second title = %q, want %q", response[1].Title, second.Title)
		}
	})
}

func TestHTTPHandlerGetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repository := NewMemoryRepository()
		service := NewService(repository)
		handler := NewHTTPHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/tasks/1", nil)

		created, err := service.Create(request.Context(), 42, "Learn Go", "Study HTTP handlers")
		if err != nil {
			t.Fatalf("Create task: %v", err)
		}
		request.SetPathValue("id", "1")
		recorder := httptest.NewRecorder()

		serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.GetByID), recorder, request, 42)

		if recorder.Code != http.StatusOK {
			t.Fatalf("GetByID() status code = %d, want %d", recorder.Code, http.StatusOK)
		}

		var response taskResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("Decode response body: %v", err)
		}
		if response.ID != created.ID {
			t.Errorf("GetByID() ID = %d, want %d", response.ID, created.ID)
		}
		if response.Title != created.Title {
			t.Errorf("GetByID() Title = %q, want %q", response.Title, created.Title)
		}
		if response.Status != created.Status {
			t.Errorf("GetByID() Status = %q, want %q", response.Status, created.Status)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		repository := NewMemoryRepository()
		service := NewService(repository)
		handler := NewHTTPHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/tasks/abc", nil)
		request.SetPathValue("id", "abc")
		recorder := httptest.NewRecorder()

		serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.GetByID), recorder, request, 42)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("GetByID() status code = %d, want %d", recorder.Code, http.StatusBadRequest)
		}

		var response errorResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("Decode response body: %v", err)
		}
		if response.Error != "invalid task id" {
			t.Errorf("GetByID() error = %q, want %q", response.Error, "invalid task id")
		}
	})

	t.Run("not found", func(t *testing.T) {
		repository := NewMemoryRepository()
		service := NewService(repository)
		handler := NewHTTPHandler(service)
		request := httptest.NewRequest(http.MethodGet, "/tasks/999", nil)
		request.SetPathValue("id", "999")
		recorder := httptest.NewRecorder()

		serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.GetByID), recorder, request, 42)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("GetByID() status code = %d, want %d", recorder.Code, http.StatusNotFound)
		}

		var response errorResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("Decode response body: %v", err)
		}
		if response.Error != ErrNotFound.Error() {
			t.Errorf("GetByID() error = %q, want %q", response.Error, ErrNotFound.Error())
		}
	})

	t.Run("request timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			repository := &timeoutRepository{Repository: NewMemoryRepository()}
			service := NewService(repository)
			handler := NewHTTPHandler(service)
			request := httptest.NewRequest(http.MethodGet, "/tasks/1", nil)
			request.SetPathValue("id", "1")
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.GetByID), recorder, request, 42)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("GetByID() status code = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("GetByID() Content-Type = %q, want %q", contentType, "application/json")
			}

			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response body: %v", err)
			}
			if response.Error != "request timed out" {
				t.Errorf("GetByID() error = %q, want %q", response.Error, "request timed out")
			}
		})
	})
}

func TestHTTPHandlerGetByIDOwnership(t *testing.T) {
	service := NewService(NewMemoryRepository())
	created, err := service.Create(t.Context(), 42, "Owner A's task", "Private description")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	handler := NewHTTPHandler(service).Routes()
	tests := []struct {
		name       string
		userID     int64
		taskID     int64
		wantStatus int
	}{
		{name: "owner A", userID: 42, taskID: created.ID, wantStatus: http.StatusOK},
		{name: "other user B", userID: 43, taskID: created.ID, wantStatus: http.StatusNotFound},
		{name: "missing task", userID: 43, taskID: created.ID + 1, wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/tasks/"+strconv.FormatInt(tt.taskID, 10), nil)
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, handler, recorder, request, tt.userID)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("GetByID() status = %d, want %d; body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if tt.wantStatus == http.StatusOK {
				var response taskResponse
				if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
					t.Fatalf("Decode task response: %v", err)
				}
				if response.ID != created.ID || response.Title != created.Title || response.Description != created.Description {
					t.Errorf("GetByID() task = %+v, want %+v", response, created)
				}
				return
			}
			var response map[string]any
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode error response: %v", err)
			}
			if len(response) != 1 || response["error"] != ErrNotFound.Error() {
				t.Errorf("Error response = %v, want only error=%q", response, ErrNotFound.Error())
			}
		})
	}
}

func TestHTTPHandlerUpdateSuccess(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository)
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(
		http.MethodPut,
		"/tasks/1",
		strings.NewReader(`{"title":"  Updated task  ","description":"  Updated description  ","status":"in_progress"}`),
	)
	created, err := service.Create(request.Context(), 42, "Original task", "Original description")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	request.SetPathValue("id", "1")
	recorder := httptest.NewRecorder()

	serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.Update), recorder, request, 42)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Update() status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Update() Content-Type = %q, want %q", contentType, "application/json")
	}

	var response taskResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response body: %v", err)
	}
	if response.ID != created.ID {
		t.Errorf("Update() ID = %d, want %d", response.ID, created.ID)
	}
	if response.Title != "Updated task" {
		t.Errorf("Update() Title = %q, want %q", response.Title, "Updated task")
	}
	if response.Description != "Updated description" {
		t.Errorf("Update() Description = %q, want %q", response.Description, "Updated description")
	}
	if response.Status != StatusInProgress {
		t.Errorf("Update() Status = %q, want %q", response.Status, StatusInProgress)
	}
	if !response.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("Update() CreatedAt = %v, want %v", response.CreatedAt, created.CreatedAt)
	}
	if response.UpdatedAt.Before(response.CreatedAt) {
		t.Errorf("Update() UpdatedAt = %v, want not before CreatedAt %v", response.UpdatedAt, response.CreatedAt)
	}

	stored, err := service.GetByID(request.Context(), 42, created.ID)
	if err != nil {
		t.Fatalf("GetByID() updated task: %v", err)
	}
	if stored.ID != response.ID {
		t.Errorf("GetByID() ID = %d, want %d", stored.ID, response.ID)
	}
	if stored.Title != response.Title {
		t.Errorf("GetByID() Title = %q, want %q", stored.Title, response.Title)
	}
	if stored.Description != response.Description {
		t.Errorf("GetByID() Description = %q, want %q", stored.Description, response.Description)
	}
	if stored.Status != response.Status {
		t.Errorf("GetByID() Status = %q, want %q", stored.Status, response.Status)
	}
	if !stored.CreatedAt.Equal(response.CreatedAt) {
		t.Errorf("GetByID() CreatedAt = %v, want %v", stored.CreatedAt, response.CreatedAt)
	}
	if !stored.UpdatedAt.Equal(response.UpdatedAt) {
		t.Errorf("GetByID() UpdatedAt = %v, want %v", stored.UpdatedAt, response.UpdatedAt)
	}
}

func TestHTTPHandlerUpdateOwnership(t *testing.T) {
	service := NewService(NewMemoryRepository())
	created, err := service.Create(t.Context(), 42, "Original task", "Original description")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	handler := NewHTTPHandler(service).Routes()
	path := "/tasks/" + strconv.FormatInt(created.ID, 10)
	const body = `{"title":"Updated task","description":"Updated description","status":"done"}`
	steps := []struct {
		name       string
		method     string
		userID     int64
		body       string
		wantStatus int
		wantTask   taskResponse
	}{
		{
			name: "user B cannot update user A's task", method: http.MethodPut,
			userID: 43, body: body, wantStatus: http.StatusNotFound,
		},
		{
			name: "user A reads unchanged task", method: http.MethodGet,
			userID: 42, wantStatus: http.StatusOK, wantTask: newTaskResponse(created),
		},
		{
			name: "user A can send the same update", method: http.MethodPut,
			userID: 42, body: body, wantStatus: http.StatusOK,
			wantTask: taskResponse{ID: created.ID, Title: "Updated task", Description: "Updated description", Status: StatusDone, CreatedAt: created.CreatedAt},
		},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) {
			request := httptest.NewRequest(step.method, path, strings.NewReader(step.body))
			recorder := httptest.NewRecorder()
			serveAuthenticatedTaskRequest(t, handler, recorder, request, step.userID)

			if recorder.Code != step.wantStatus {
				t.Fatalf("Status = %d, want %d; body = %s", recorder.Code, step.wantStatus, recorder.Body.String())
			}
			if step.wantStatus == http.StatusNotFound {
				var response map[string]any
				if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
					t.Fatalf("Decode error response: %v", err)
				}
				if len(response) != 1 || response["error"] != ErrNotFound.Error() {
					t.Errorf("Error response = %v, want only error=%q", response, ErrNotFound.Error())
				}
				return
			}
			var response taskResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode task response: %v", err)
			}
			want := step.wantTask
			if response.ID != want.ID || response.Title != want.Title || response.Description != want.Description || response.Status != want.Status || !response.CreatedAt.Equal(want.CreatedAt) {
				t.Errorf("Task = %+v, want %+v", response, want)
			}
			if step.method == http.MethodGet && !response.UpdatedAt.Equal(created.UpdatedAt) {
				t.Errorf("Foreign update changed UpdatedAt: got %v, want %v", response.UpdatedAt, created.UpdatedAt)
			}
		}) {
			return
		}
	}
}

func TestHTTPHandlerUpdateUnauthorized(t *testing.T) {
	handler := NewHTTPHandler(nil)
	request := httptest.NewRequest(http.MethodPut, "/tasks/1", strings.NewReader(`{"title":"Updated","status":"done"}`))
	request.SetPathValue("id", "1")
	recorder := httptest.NewRecorder()

	handler.Update(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Update() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
	var response errorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Decode error response: %v", err)
	}
	if response.Error != "unauthorized" {
		t.Errorf("Error = %q, want unauthorized", response.Error)
	}
}

func TestHTTPHandlerUpdateErrors(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid id format",
			id:         "abc",
			body:       `{"title":"Updated","description":"Test","status":"done"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task id",
		},
		{
			name:       "non-positive id",
			id:         "0",
			body:       `{"title":"Updated","description":"Test","status":"done"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task id",
		},
		{
			name:       "broken JSON",
			id:         "1",
			body:       `{"title":`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "unknown field",
			id:         "1",
			body:       `{"title":"Updated","description":"Test","status":"done","priority":1}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "empty title",
			id:         "1",
			body:       `{"title":"   ","description":"Test","status":"done"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  ErrTitleRequired.Error(),
		},
		{
			name:       "invalid status",
			id:         "1",
			body:       `{"title":"Updated","description":"Test","status":"unknown"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  ErrInvalidStatus.Error(),
		},
		{
			name:       "task not found",
			id:         "999",
			body:       `{"title":"Updated","description":"Test","status":"done"}`,
			wantStatus: http.StatusNotFound,
			wantError:  ErrNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := NewMemoryRepository()
			service := NewService(repository)
			handler := NewHTTPHandler(service)

			request := httptest.NewRequest(
				http.MethodPut,
				"/tasks/"+tt.id,
				strings.NewReader(tt.body),
			)
			created, err := service.Create(request.Context(), 42, "Original task", "Original description")
			if err != nil {
				t.Fatalf("Create task: %v", err)
			}
			request.SetPathValue("id", tt.id)
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.Update), recorder, request, 42)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("Update() status code = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Update() Content-Type = %q, want %q", contentType, "application/json")
			}

			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response body: %v", err)
			}
			if response.Error != tt.wantError {
				t.Errorf("Update() error = %q, want %q", response.Error, tt.wantError)
			}

			stored, err := service.GetByID(request.Context(), 42, 1)
			if err != nil {
				t.Fatalf("GetByID() original task: %v", err)
			}
			if stored != created {
				t.Errorf("GetByID() task = %+v, want unchanged %+v", stored, created)
			}
		})
	}
}

func TestHTTPHandlerDeleteSuccess(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository)
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(http.MethodDelete, "/tasks/1", nil)
	created, err := service.Create(request.Context(), 42, "Task to delete", "Description")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	request.SetPathValue("id", "1")
	recorder := httptest.NewRecorder()

	serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.Delete), recorder, request, 42)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("Delete() status code = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("Delete() response body = %q, want empty", recorder.Body.String())
	}

	_, err = service.GetByID(request.Context(), 42, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID() error = %v, want %v", err, ErrNotFound)
	}
}

func TestHTTPHandlerDeleteOwnership(t *testing.T) {
	service := NewService(NewMemoryRepository())
	created, err := service.Create(t.Context(), 42, "User A's task", "Private description")
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}
	handler := NewHTTPHandler(service).Routes()
	path := "/tasks/" + strconv.FormatInt(created.ID, 10)
	steps := []struct {
		name       string
		method     string
		userID     int64
		wantStatus int
	}{
		{name: "user B cannot delete user A's task", method: http.MethodDelete, userID: 43, wantStatus: http.StatusNotFound},
		{name: "user A can still read the task", method: http.MethodGet, userID: 42, wantStatus: http.StatusOK},
		{name: "user A deletes the task", method: http.MethodDelete, userID: 42, wantStatus: http.StatusNoContent},
		{name: "user A cannot read the deleted task", method: http.MethodGet, userID: 42, wantStatus: http.StatusNotFound},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) {
			request := httptest.NewRequest(step.method, path, nil)
			recorder := httptest.NewRecorder()
			serveAuthenticatedTaskRequest(t, handler, recorder, request, step.userID)

			if recorder.Code != step.wantStatus {
				t.Fatalf("Status = %d, want %d; body = %s", recorder.Code, step.wantStatus, recorder.Body.String())
			}
			switch step.wantStatus {
			case http.StatusNotFound:
				var response map[string]any
				if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
					t.Fatalf("Decode error response: %v", err)
				}
				if len(response) != 1 || response["error"] != ErrNotFound.Error() {
					t.Errorf("Error response = %v, want only error=%q", response, ErrNotFound.Error())
				}
			case http.StatusOK:
				var response taskResponse
				if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
					t.Fatalf("Decode task response: %v", err)
				}
				if want := newTaskResponse(created); response != want {
					t.Errorf("Task = %+v, want unchanged %+v", response, want)
				}
			case http.StatusNoContent:
				if recorder.Body.Len() != 0 {
					t.Errorf("Delete() response body = %q, want empty", recorder.Body.String())
				}
			}
		}) {
			return
		}
	}
}

func TestHTTPHandlerDeleteUnauthorized(t *testing.T) {
	handler := NewHTTPHandler(nil)
	request := httptest.NewRequest(http.MethodDelete, "/tasks/1", nil)
	request.SetPathValue("id", "1")
	recorder := httptest.NewRecorder()

	handler.Delete(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Delete() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}
	var response errorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("Decode error response: %v", err)
	}
	if response.Error != "unauthorized" {
		t.Errorf("Error = %q, want unauthorized", response.Error)
	}
}

func TestHTTPHandlerDeleteErrors(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid id format",
			id:         "abc",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task id",
		},
		{
			name:       "non-positive id",
			id:         "0",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task id",
		},
		{
			name:       "task not found",
			id:         "999",
			wantStatus: http.StatusNotFound,
			wantError:  ErrNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := NewMemoryRepository()
			service := NewService(repository)
			handler := NewHTTPHandler(service)

			created, err := service.Create(t.Context(), 42, "Original task", "Original description")
			if err != nil {
				t.Fatalf("Create task: %v", err)
			}
			request := httptest.NewRequest(http.MethodDelete, "/tasks/"+tt.id, nil)
			request.SetPathValue("id", tt.id)
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, http.HandlerFunc(handler.Delete), recorder, request, 42)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("Delete() status code = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Delete() Content-Type = %q, want %q", contentType, "application/json")
			}

			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response body: %v", err)
			}
			if response.Error != tt.wantError {
				t.Errorf("Delete() error = %q, want %q", response.Error, tt.wantError)
			}

			stored, err := service.GetByID(request.Context(), 42, 1)
			if err != nil {
				t.Fatalf("GetByID() original task: %v", err)
			}
			if stored != created {
				t.Errorf("GetByID() task = %+v, want unchanged %+v", stored, created)
			}
		})
	}
}

func TestHTTPHandlerOperationErrors(t *testing.T) {
	operations := []struct {
		name   string
		method string
		body   string
		handle func(*HTTPHandler, http.ResponseWriter, *http.Request)
	}{
		{
			name:   "Create",
			method: http.MethodPost,
			body:   `{"title":"New task","description":"Test"}`,
			handle: (*HTTPHandler).Create,
		},
		{
			name:   "List",
			method: http.MethodGet,
			handle: (*HTTPHandler).List,
		},
		{
			name:   "Update",
			method: http.MethodPut,
			body:   `{"title":"Updated","description":"Test","status":"done"}`,
			handle: (*HTTPHandler).Update,
		},
		{
			name:   "Delete",
			method: http.MethodDelete,
			handle: (*HTTPHandler).Delete,
		},
	}
	failures := []struct {
		name       string
		canceled   bool
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name:       "request timeout",
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "request timed out",
		},
		{
			name:     "request canceled",
			canceled: true,
		},
		{
			name:       "repository error",
			err:        errors.New("repository unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, failure := range failures {
				t.Run(failure.name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						var repository Repository = &timeoutRepository{}
						if failure.err != nil {
							repository = &errorRepository{err: failure.err}
						}
						handler := NewHTTPHandler(NewService(repository))
						request := httptest.NewRequest(operation.method, "/tasks/1", strings.NewReader(operation.body))
						request.SetPathValue("id", "1")
						if failure.canceled {
							ctx, cancel := context.WithCancel(request.Context())
							cancel()
							request = request.WithContext(ctx)
						}
						recorder := httptest.NewRecorder()
						recorder.Code = 0
						started := time.Now()

						serveAuthenticatedTaskRequest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							operation.handle(handler, w, r)
						}), recorder, request, 42)

						if failure.canceled {
							if recorder.Code != 0 || recorder.Body.Len() != 0 || len(recorder.Header()) != 0 {
								t.Errorf("Canceled request wrote a response: status = %d, headers = %v, body = %q", recorder.Code, recorder.Header(), recorder.Body.String())
							}
							return
						}
						if failure.err == nil && time.Since(started) != 3*time.Second {
							t.Errorf("Operation timeout = %v, want %v", time.Since(started), 3*time.Second)
						}
						if recorder.Code != failure.wantStatus {
							t.Fatalf("Status code = %d, want %d", recorder.Code, failure.wantStatus)
						}
						if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
							t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
						}
						var response errorResponse
						if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
							t.Fatalf("Decode response body: %v", err)
						}
						if response.Error != failure.wantError {
							t.Errorf("Error = %q, want %q", response.Error, failure.wantError)
						}
					})
				})
			}
		})
	}
}

func TestHTTPHandlerRoutes(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		createTask bool
		wantStatus int
	}{
		{
			name:       "create route",
			method:     http.MethodPost,
			path:       "/tasks",
			body:       `{"title":"New task","description":"Test"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "list route",
			method:     http.MethodGet,
			path:       "/tasks",
			wantStatus: http.StatusOK,
		},
		{
			name:       "get by id route",
			method:     http.MethodGet,
			path:       "/tasks/1",
			createTask: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "update route",
			method:     http.MethodPut,
			path:       "/tasks/1",
			body:       `{"title":"Updated","description":"Test","status":"done"}`,
			createTask: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete route",
			method:     http.MethodDelete,
			path:       "/tasks/1",
			createTask: true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "method not allowed",
			method:     http.MethodPatch,
			path:       "/tasks/1",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "unknown route",
			method:     http.MethodGet,
			path:       "/unknown",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := NewMemoryRepository()
			service := NewService(repository)
			handler := NewHTTPHandler(service)
			router := handler.Routes()

			if tt.createTask {
				if _, err := service.Create(t.Context(), 42, "Original task", "Original description"); err != nil {
					t.Fatalf("Create task: %v", err)
				}
			}

			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, router, recorder, request, 42)

			if recorder.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status code = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

func TestHTTPHandlerListPagination(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	createdIDs := make([]int64, 0, 25)
	for i := 0; i < 25; i++ {
		created, err := repository.Create(ctx, Task{UserID: 42, Title: "Task", Status: StatusTodo})
		if err != nil {
			t.Fatalf("Create() task %d error = %v", i+1, err)
		}
		createdIDs = append(createdIDs, created.ID)
	}
	service := NewService(repository)
	handler := NewHTTPHandler(service)

	tests := []struct {
		name    string
		url     string
		wantIDs []int64
	}{
		{name: "default page", url: "/tasks", wantIDs: createdIDs[:20]},
		{name: "limit and offset", url: "/tasks?limit=2&offset=2", wantIDs: createdIDs[2:4]},
		{name: "last partial page", url: "/tasks?limit=2&offset=24", wantIDs: createdIDs[24:]},
		{name: "offset at end", url: "/tasks?offset=25", wantIDs: createdIDs[25:]},
		{name: "limit only", url: "/tasks?limit=1", wantIDs: createdIDs[:1]},
		{name: "offset only", url: "/tasks?offset=24", wantIDs: createdIDs[24:]},
		{name: "offset within page", url: "/tasks?limit=2&offset=1", wantIDs: createdIDs[1:3]},
		{name: "maximum limit", url: "/tasks?limit=100", wantIDs: createdIDs},
		{name: "offset past end", url: "/tasks?offset=100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.url, nil)
			recorder := httptest.NewRecorder()
			serveAuthenticatedTaskRequest(t, handler.Routes(), recorder, request, 42)

			if recorder.Code != http.StatusOK {
				t.Fatalf("List() status code = %d, want %d", recorder.Code, http.StatusOK)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("List() Content-Type = %q, want application/json", contentType)
			}

			var response []taskResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response body: %v", err)
			}
			if response == nil {
				t.Fatal("List() response is nil, want non-nil slice")
			}
			if len(response) != len(tt.wantIDs) {
				t.Fatalf("List() response length = %d, want %d", len(response), len(tt.wantIDs))
			}
			for i, wantID := range tt.wantIDs {
				if response[i].ID != wantID {
					t.Errorf("List() response[%d].ID = %d, want %d", i, response[i].ID, wantID)
				}
			}
		})
	}
}

func TestHTTPHandlerListStatusFilter(t *testing.T) {
	ctx := context.Background()
	repository := NewMemoryRepository()
	createdIDs := make([]int64, 0, 4)
	for _, task := range []Task{
		{UserID: 42, Title: "A", Status: StatusTodo},
		{UserID: 42, Title: "B", Status: StatusDone},
		{UserID: 42, Title: "C", Status: StatusInProgress},
		{UserID: 42, Title: "D", Status: StatusDone},
	} {
		created, err := repository.Create(ctx, task)
		if err != nil {
			t.Fatalf("Create() task %q error = %v", task.Title, err)
		}
		createdIDs = append(createdIDs, created.ID)
	}
	handler := NewHTTPHandler(NewService(repository))
	router := handler.Routes()

	tests := []struct {
		name       string
		url        string
		wantStatus int
		wantIDs    []int64
		wantError  string
	}{
		{
			name:       "done",
			url:        "/tasks?status=done",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[1], createdIDs[3]},
		},
		{
			name:       "done with pagination",
			url:        "/tasks?status=done&limit=1&offset=1",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[3]},
		},
		{
			name:       "todo",
			url:        "/tasks?status=todo",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[0]},
		},
		{
			name:       "in progress",
			url:        "/tasks?status=in_progress",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[2]},
		},
		{
			name:       "empty status",
			url:        "/tasks?status=",
			wantStatus: http.StatusOK,
			wantIDs:    createdIDs,
		},
		{
			name:       "invalid status",
			url:        "/tasks?status=unknown",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid task status",
		},
		{
			name:       "ascending order",
			url:        "/tasks?order=asc",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[0], createdIDs[1], createdIDs[2], createdIDs[3]},
		},
		{
			name:       "empty order",
			url:        "/tasks?order=",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[0], createdIDs[1], createdIDs[2], createdIDs[3]},
		},
		{
			name:       "descending order",
			url:        "/tasks?order=desc",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[3], createdIDs[2], createdIDs[1], createdIDs[0]},
		},
		{
			name:       "descending order with pagination",
			url:        "/tasks?order=desc&limit=2&offset=1",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[2], createdIDs[1]},
		},
		{
			name:       "done with descending order and pagination",
			url:        "/tasks?status=done&order=desc&limit=1&offset=1",
			wantStatus: http.StatusOK,
			wantIDs:    []int64{createdIDs[1]},
		},
		{
			name:       "invalid order",
			url:        "/tasks?order=unknown",
			wantStatus: http.StatusBadRequest,
			wantError:  ErrInvalidOrder.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.url, nil)
			recorder := httptest.NewRecorder()
			serveAuthenticatedTaskRequest(t, router, recorder, request, 42)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("List() status code = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("List() Content-Type = %q, want application/json", contentType)
			}

			if tt.wantError != "" {
				var response errorResponse
				if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
					t.Fatalf("Decode error response: %v", err)
				}
				if response.Error != tt.wantError {
					t.Errorf("List() error = %q, want %q", response.Error, tt.wantError)
				}
				return
			}

			var response []taskResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response body: %v", err)
			}
			if len(response) != len(tt.wantIDs) {
				t.Fatalf("List() response length = %d, want %d", len(response), len(tt.wantIDs))
			}
			for i, wantID := range tt.wantIDs {
				if response[i].ID != wantID {
					t.Errorf("List() response[%d].ID = %d, want %d", i, response[i].ID, wantID)
				}
			}
		})
	}
}

func TestHTTPHandlerListInvalidPagination(t *testing.T) {
	service := NewService(NewMemoryRepository())
	handler := NewHTTPHandler(service)

	tests := []struct {
		name      string
		url       string
		wantError string
	}{
		{name: "malformed limit", url: "/tasks?limit=abc", wantError: "invalid limit"},
		{name: "empty limit", url: "/tasks?limit=", wantError: "invalid limit"},
		{name: "zero limit", url: "/tasks?limit=0", wantError: ErrInvalidPagination.Error()},
		{name: "limit too large", url: "/tasks?limit=101", wantError: ErrInvalidPagination.Error()},
		{name: "malformed offset", url: "/tasks?offset=abc", wantError: "invalid offset"},
		{name: "empty offset", url: "/tasks?offset=", wantError: "invalid offset"},
		{name: "negative offset", url: "/tasks?offset=-1", wantError: ErrInvalidPagination.Error()},
		{name: "negative limit", url: "/tasks?limit=-1", wantError: ErrInvalidPagination.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.url, nil)
			recorder := httptest.NewRecorder()
			serveAuthenticatedTaskRequest(t, handler.Routes(), recorder, request, 42)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("List() status code = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("List() Content-Type = %q, want application/json", contentType)
			}

			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode error response: %v", err)
			}
			if response.Error != tt.wantError {
				t.Errorf("List() error = %q, want %q", response.Error, tt.wantError)
			}
		})
	}
}

func TestHTTPHandlerListUnauthorized(t *testing.T) {
	for _, path := range []string{"/tasks", "/tasks?limit=bad", "/tasks?status=invalid"} {
		t.Run(path, func(t *testing.T) {
			handler := NewHTTPHandler(nil)
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()

			handler.List(recorder, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("List() status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var response errorResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response: %v", err)
			}
			if response.Error != "unauthorized" {
				t.Errorf("Error = %q, want unauthorized", response.Error)
			}
		})
	}
}

func TestHTTPHandlerListOwnership(t *testing.T) {
	repository := NewMemoryRepository()
	created := make([]Task, 0, 3)
	for _, input := range []Task{
		{UserID: 42, Title: "A1", Status: StatusDone},
		{UserID: 43, Title: "B1", Status: StatusDone},
		{UserID: 42, Title: "A2", Status: StatusDone},
	} {
		task, err := repository.Create(t.Context(), input)
		if err != nil {
			t.Fatalf("Create %s: %v", input.Title, err)
		}
		created = append(created, task)
	}
	handler := NewHTTPHandler(NewService(repository)).Routes()
	tests := []struct {
		name    string
		userID  int64
		path    string
		wantIDs []int64
	}{
		{
			name: "owner A without status", userID: 42, path: "/tasks",
			wantIDs: []int64{created[0].ID, created[2].ID},
		},
		{
			name: "owner A with status", userID: 42, path: "/tasks?status=done",
			wantIDs: []int64{created[0].ID, created[2].ID},
		},
		{
			name: "owner before pagination", userID: 42,
			path:    "/tasks?status=done&order=asc&limit=1&offset=1",
			wantIDs: []int64{created[2].ID},
		},
		{
			name: "owner comes from JWT", userID: 42,
			path:    "/tasks?user_id=43&status=done&order=asc&limit=1&offset=1",
			wantIDs: []int64{created[2].ID},
		},
		{
			name: "owner B", userID: 43, path: "/tasks?status=done",
			wantIDs: []int64{created[1].ID},
		},
		{name: "owner without tasks", userID: 44, path: "/tasks"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()

			serveAuthenticatedTaskRequest(t, handler, recorder, request, tt.userID)

			if recorder.Code != http.StatusOK {
				t.Fatalf("List() status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			var response []taskResponse
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response: %v", err)
			}
			if response == nil {
				t.Fatal("List() returned null, want a JSON array")
			}
			if len(response) != len(tt.wantIDs) {
				t.Fatalf("List() tasks = %+v, want IDs %v", response, tt.wantIDs)
			}
			for i, wantID := range tt.wantIDs {
				if response[i].ID != wantID {
					t.Errorf("List() task %d ID = %d, want %d", i, response[i].ID, wantID)
				}
			}
		})
	}
}
