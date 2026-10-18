package task

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type HTTPHandler struct {
	service *Service
}

func NewHTTPHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{
		service: service,
	}
}

func (h *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /tasks", h.Create)
	mux.HandleFunc("GET /tasks", h.List)
	mux.HandleFunc("GET /tasks/{id}", h.GetByID)
	mux.HandleFunc("PUT /tasks/{id}", h.Update)
	mux.HandleFunc("DELETE /tasks/{id}", h.Delete)

	return mux
}

func newTaskResponse(task Task) taskResponse {
	return taskResponse{
		ID:          task.ID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func (h *HTTPHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request createTaskRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	created, err := h.service.Create(ctx, request.Title, request.Description)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")
			return
		}

		if errors.Is(err, ErrTitleRequired) {
			_ = writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, newTaskResponse(created))
}

func (h *HTTPHandler) List(w http.ResponseWriter, r *http.Request) {
	options := ListOptions{
		Limit:  20,
		Offset: 0,
		Order:  "",
	}

	query := r.URL.Query()
	options.Status = TaskStatus(query.Get("status"))
	if query.Has("limit") {
		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil {
			_ = writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		options.Limit = limit
	}

	if query.Has("offset") {
		offset, err := strconv.Atoi(query.Get("offset"))
		if err != nil {
			_ = writeError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		options.Offset = offset
	}
	if query.Has("order") {
		options.Order = query.Get("order")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	tasks, err := h.service.List(ctx, options)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return

		case errors.Is(err, context.DeadlineExceeded):
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")

		case errors.Is(err, ErrInvalidPagination):
			_ = writeError(w, http.StatusBadRequest, ErrInvalidPagination.Error())
		case errors.Is(err, ErrInvalidStatus):
			_ = writeError(w, http.StatusBadRequest, ErrInvalidStatus.Error())
		case errors.Is(err, ErrInvalidOrder):
			_ = writeError(w, http.StatusBadRequest, ErrInvalidOrder.Error())
		default:
			_ = writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	_ = writeJSON(w, http.StatusOK, newTaskListResponse(tasks))
}

func (h *HTTPHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idText := r.PathValue("id")

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		_ = writeError(w, http.StatusBadRequest, "invalid task id")
		return

	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	task, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")
			return
		}

		if errors.Is(err, ErrNotFound) {
			_ = writeError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}

		_ = writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, newTaskResponse(task))
}

func (h *HTTPHandler) Update(w http.ResponseWriter, r *http.Request) {
	idText := r.PathValue("id")

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		_ = writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	var request updateTaskRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		_ = writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	task, err := h.service.Update(ctx, id, request.Title, request.Description, request.Status)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")
			return
		}

		switch {
		case errors.Is(err, ErrTitleRequired),
			errors.Is(err, ErrInvalidStatus):
			_ = writeError(w, http.StatusBadRequest, err.Error())

		case errors.Is(err, ErrNotFound):
			_ = writeError(w, http.StatusNotFound, ErrNotFound.Error())

		default:
			_ = writeError(
				w,
				http.StatusInternalServerError,
				"internal server error",
			)
		}

		return
	}
	_ = writeJSON(w, http.StatusOK, newTaskResponse(task))
}

func (h *HTTPHandler) Delete(w http.ResponseWriter, r *http.Request) {

	idText := r.PathValue("id")
	id, err := strconv.ParseInt(idText, 10, 64)

	if err != nil || id <= 0 {
		_ = writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err = h.service.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			_ = writeError(w, http.StatusServiceUnavailable, "request timed out")
			return
		}

		if errors.Is(err, ErrNotFound) {
			_ = writeError(w, http.StatusNotFound, ErrNotFound.Error())
			return
		}
		_ = writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
