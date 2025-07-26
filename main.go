package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// --- STYLES ---
var (
	appStyle           = lipgloss.NewStyle().Padding(1, 2)
	titleStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFDF5")).Background(lipgloss.Color("#25A065")).Padding(0, 1)
	statusMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).Render
	focusedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle        = focusedStyle
	noStyle            = lipgloss.NewStyle()
	helpStyle          = blurredStyle
)

// --- CLICKUP API ---
type Task struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"description"`
	Status  struct {
		Status string `json:"status"`
	} `json:"status"`
	Space struct {
		ID string `json:"id"`
	} `json:"space"`
	List struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"list"`
	Folder struct {
		Name string `json:"name"`
	} `json:"folder"`
}

type TasksResponse struct {
	Tasks []Task `json:"tasks"`
}

type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s Space) FilterValue() string { return s.Name }
func (s Space) Title() string       { return s.Name }
func (s Space) Description() string { return "" }

type SpacesResponse struct {
	Spaces []Space `json:"spaces"`
}

type ListInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (l ListInfo) FilterValue() string { return l.Name }
func (l ListInfo) Title() string       { return l.Name }
func (l ListInfo) Description() string { return "" }

type ListsResponse struct {
	Lists []ListInfo `json:"lists"`
}

type Folder struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Lists []ListInfo `json:"lists"`
}

type FoldersResponse struct {
	Folders []Folder `json:"folders"`
}

type Status struct {
	Status string `json:"status"`
	Order  int    `json:"orderindex"`
	Color  string `json:"color"`
}

type Comment struct {
	ID      string `json:"id"`
	Comment []struct {
		Text string `json:"text"`
	} `json:"comment"`
	Date string `json:"date"`
	User struct {
		Username string `json:"username"`
	} `json:"user"`
}

type CommentsResponse struct {
	Comments []Comment `json:"comments"`
}

type Member struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

func (m Member) FilterValue() string { return m.Username }
func (m Member) Title() string       { return m.Username }
func (m Member) Description() string { return m.Email }

type MembersResponse struct {
	Members []Member `json:"members"`
}

func (s Status) FilterValue() string { return s.Status }
func (s Status) Title() string       { return s.Status }
func (s Status) Description() string { return "" }

type StatusesResponse struct {
	Statuses []Status `json:"statuses"`
}

type Priority struct {
	Name  string
	Value int
	Color string
}

func (p Priority) FilterValue() string { return p.Name }
func (p Priority) Title() string       { return p.Name }
func (p Priority) Description() string { return "" }

var priorities = []list.Item{
	Priority{Name: "Urgent", Value: 1, Color: "#ff0000"},
	Priority{Name: "High", Value: 2, Color: "#ff8000"},
	Priority{Name: "Normal", Value: 3, Color: "#00aaff"},
	Priority{Name: "Low", Value: 4, Color: "#d3d3d3"},
	Priority{Name: "None", Value: 0, Color: "#ffffff"},
}

func (t Task) FilterValue() string { return t.Name }
func (t Task) Title() string       { return t.Name }
func (t Task) Description() string {
	return fmt.Sprintf("In: %s / %s | Status: %s", t.Folder.Name, t.List.Name, t.Status.Status)
}

// --- CUSTOM LIST DELEGATES ---
type statusDelegate struct{}

func (d statusDelegate) Height() int                               { return 1 }
func (d statusDelegate) Spacing() int                              { return 0 }
func (d statusDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d statusDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	s, ok := listItem.(Status)
	if !ok {
		return
	}
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(s.Color))
	statusName := s.Status
	if index == m.Index() {
		fmt.Fprint(w, statusStyle.Render("> "+statusName))
	} else {
		fmt.Fprint(w, statusStyle.Render("  "+statusName))
	}
}

type assigneeDelegate struct {
	selected map[int]struct{}
}

func (d assigneeDelegate) Height() int                               { return 1 }
func (d assigneeDelegate) Spacing() int                              { return 0 }
func (d assigneeDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d assigneeDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	member, ok := listItem.(Member)
	if !ok {
		return
	}
	isSelected := "[ ]"
	if _, exists := d.selected[member.ID]; exists {
		isSelected = "[x]"
	}
	line := fmt.Sprintf("%s %s", isSelected, member.Username)
	if index == m.Index() {
		fmt.Fprint(w, focusedStyle.Render("> "+line))
	} else {
		fmt.Fprint(w, "  "+line)
	}
}

type priorityDelegate struct{}

func (d priorityDelegate) Height() int                               { return 1 }
func (d priorityDelegate) Spacing() int                              { return 0 }
func (d priorityDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d priorityDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	p, ok := listItem.(Priority)
	if !ok {
		return
	}
	priorityStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.Color))
	priorityName := p.Name
	if index == m.Index() {
		fmt.Fprint(w, priorityStyle.Render("> "+priorityName))
	} else {
		fmt.Fprint(w, priorityStyle.Render("  "+priorityName))
	}
}

// --- MODEL ---

type viewState int

const (
	formView viewState = iota
	spaceSelectionView
	listView
	statusUpdateView
	taskDetailView
	editTaskView
	listSelectionView
	createTaskTitleView
	createTaskDescView
	createTaskStatusView
	createTaskAssigneeView
	createTaskPriorityView
	taskCreatedView
	deleteConfirmationView
	taskDeletedView
)

const (
	titleFocus int = iota
	descriptionFocus
	statusFocus
	assigneeFocus
	priorityFocus
)

type model struct {
	state             viewState
	err               error
	quitting          bool
	width, height     int
	loading           bool
	insertMode        bool
	commandMode       bool
	isCreatingTask    bool
	spinner           spinner.Model
	progress          progress.Model
	focusIndex        int
	createFocusIndex  int
	inputs            []textinput.Model
	list              list.Model
	spaceList         list.Model
	folderlessList    list.Model
	statusList        list.Model
	assigneeList      list.Model
	priorityList      list.Model
	viewport          viewport.Model
	descriptionBox    textarea.Model
	commentBox        textinput.Model
	commandInput      textinput.Model
	titleInput        textinput.Model
	statusMessage     string
	apiToken          string
	teamID            string
	spaceID           string
	listID            string
	newTaskTitle      string
	newTaskDesc       string
	newTaskStatus     string
	newTaskAssignees  []int
	newTaskPriority   int
	selectedTask      Task
	selectedStatus    string
	selectedAssignees map[int]struct{}
	comments          []Comment
	commentsLoaded    bool
	allLists          []list.Item
}

func newModel(apiToken, teamID string, creatingTask bool) model {
	if apiToken == "" || teamID == "" {
		m := model{
			state:  formView,
			inputs: make([]textinput.Model, 2),
		}
		var t textinput.Model
		for i := range m.inputs {
			t = textinput.New()
			t.Cursor.Style = cursorStyle
			t.CharLimit = 128
			switch i {
			case 0:
				t.Placeholder = "ClickUp API Token"
				t.Focus()
				t.PromptStyle = focusedStyle
			case 1:
				t.Placeholder = "Team ID"
			}
			m.inputs[i] = t
		}
		return m
	}

	sl := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	sl.Title = "Select a Space"
	return model{
		state:             spaceSelectionView,
		spaceList:         sl,
		apiToken:          apiToken,
		teamID:            teamID,
		isCreatingTask:    creatingTask,
		selectedAssignees: make(map[int]struct{}),
	}
}

// --- COMMANDS ---

func fetchSpacesCmd(apiToken, teamID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/team/%s/space?archived=false", teamID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch spaces failed: %s", string(body))
		}
		var spacesResponse SpacesResponse
		if err := json.Unmarshal(body, &spacesResponse); err != nil {
			return err
		}
		return spacesResponse
	}
}

func fetchFolderlessListsCmd(apiToken, spaceID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/space/%s/list", spaceID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch lists failed: %s", string(body))
		}
		var listsResponse ListsResponse
		if err := json.Unmarshal(body, &listsResponse); err != nil {
			return err
		}
		return listsResponse
	}
}

func fetchFoldersWithListsCmd(apiToken, spaceID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/space/%s/folder?archived=false", spaceID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch folders failed: %s", string(body))
		}
		var foldersResponse FoldersResponse
		if err := json.Unmarshal(body, &foldersResponse); err != nil {
			return err
		}
		return foldersResponse
	}
}

func fetchAssigneesCmd(apiToken, listID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/list/%s/member", listID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch assignees failed: %s", string(body))
		}
		var membersResponse MembersResponse
		if err := json.Unmarshal(body, &membersResponse); err != nil {
			return err
		}
		return membersResponse
	}
}

func createTaskCmd(apiToken, listID, name, description, status string, assignees []int, priority int) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/list/%s/task", listID)
		payloadMap := map[string]interface{}{
			"name":        name,
			"description": description,
			"status":      status,
			"assignees":   assignees,
		}
		if priority > 0 {
			payloadMap["priority"] = priority
		}
		payload, err := json.Marshal(payloadMap)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("create task failed: %s", string(body))
		}
		return "create_success"
	}
}

func fetchAllTasksCmd(apiToken, teamID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/team/%s/task", teamID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch all tasks failed: %s", string(body))
		}
		var tasksResponse TasksResponse
		if err := json.Unmarshal(body, &tasksResponse); err != nil {
			return err
		}
		return tasksResponse
	}
}

func fetchTasksCmd(apiToken, teamID, spaceID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/team/%s/task?space_ids[]=%s", teamID, spaceID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch tasks failed: %s", string(body))
		}
		var tasksResponse TasksResponse
		if err := json.Unmarshal(body, &tasksResponse); err != nil {
			return err
		}
		items := make([]list.Item, len(tasksResponse.Tasks))
		for i, task := range tasksResponse.Tasks {
			items[i] = task
		}
		return items
	}
}

func fetchTaskDetailsCmd(apiToken, taskID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/task/%s", taskID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch task details failed: %s", string(body))
		}
		var task Task
		if err := json.Unmarshal(body, &task); err != nil {
			return err
		}
		return task
	}
}

func fetchCommentsCmd(apiToken, taskID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/task/%s/comment", taskID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch comments failed: %s", string(body))
		}
		var commentsResponse CommentsResponse
		if err := json.Unmarshal(body, &commentsResponse); err != nil {
			return err
		}
		return commentsResponse
	}
}

func fetchStatusesCmd(apiToken, spaceID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/space/%s", spaceID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch statuses failed: %s", string(body))
		}
		var spaceResponse struct {
			Statuses []Status `json:"statuses"`
		}
		if err := json.Unmarshal(body, &spaceResponse); err != nil {
			return err
		}
		if len(spaceResponse.Statuses) == 0 {
			return fmt.Errorf("no statuses found for space %s", spaceID)
		}
		return StatusesResponse{Statuses: spaceResponse.Statuses}
	}
}

func postCommentCmd(apiToken, taskID, commentText string) tea.Cmd {
	return func() tea.Msg {
		if commentText == "" {
			return nil
		}
		url := fmt.Sprintf("https://api.clickup.com/api/v2/task/%s/comment", taskID)
		payload := []byte(fmt.Sprintf(`{"comment_text": "%s"}`, commentText))
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("post comment failed: %s", string(body))
		}
		return "refresh_list_success"
	}
}

func updateTaskCmd(apiToken, taskID, description, status string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/task/%s", taskID)
		payloadMap := make(map[string]string)
		if description != "" {
			payloadMap["description"] = description
		}
		if status != "" {
			payloadMap["status"] = status
		}
		if len(payloadMap) == 0 {
			return nil
		}
		payload, err := json.Marshal(payloadMap)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("PUT", url, bytes.NewBuffer(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("update task failed: %s", string(body))
		}
		return "refresh_list_success"
	}
}

func deleteTaskCmd(apiToken, taskID string) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("https://api.clickup.com/api/v2/task/%s", taskID)
		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", apiToken)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("delete task failed: %s", string(body))
		}
		return "delete_success"
	}
}

func saveCredentialsCmd(apiToken, teamID string) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "." // Fallback to current directory
		}
		filePath := filepath.Join(home, ".clup.env")
		content := fmt.Sprintf("CLICKUP_API_TOKEN=\"%s\"\nCLICKUP_TEAM_ID=\"%s\"\n", apiToken, teamID)
		err = os.WriteFile(filePath, []byte(content), 0600)
		if err != nil {
			return err
		}
		return nil
	}
}

// --- TEA PROGRAM ---

type tickMsg time.Time

func (m model) Init() tea.Cmd {
	switch m.state {
	case spaceSelectionView:
		return fetchSpacesCmd(m.apiToken, m.teamID)
	case listView:
		return fetchTasksCmd(m.apiToken, m.teamID, m.spaceID)
	case listSelectionView:
		return tea.Batch(
			fetchFolderlessListsCmd(m.apiToken, m.spaceID),
			fetchFoldersWithListsCmd(m.apiToken, m.spaceID),
		)
	default:
		return textinput.Blink
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		}
	}

	switch m.state {
	case formView:
		return updateForm(msg, m)
	case spaceSelectionView:
		return updateSpaceSelection(msg, m)
	case listSelectionView:
		return updateListSelection(msg, m)
	case createTaskTitleView:
		return updateCreateTaskTitle(msg, m)
	case createTaskDescView:
		return updateCreateTaskDesc(msg, m)
	case createTaskStatusView:
		return updateCreateTaskStatus(msg, m)
	case createTaskAssigneeView:
		return updateCreateTaskAssignee(msg, m)
	case createTaskPriorityView:
		return updateCreateTaskPriority(msg, m)
	case taskCreatedView:
		return updateTaskCreated(msg, m)
	case listView:
		return updateList(msg, m)
	case statusUpdateView:
		return updateStatusUpdate(msg, m)
	case taskDetailView:
		return updateTaskDetail(msg, m)
	case editTaskView:
		return updateEditTask(msg, m)
	case deleteConfirmationView:
		return updateDeleteConfirmation(msg, m)
	case taskDeletedView:
		return updateTaskDeleted(msg, m)
	}
	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\nAn error occurred: %v\n\nPress any key to quit.", m.err)
	}
	if m.quitting {
		return "Quitting...\n"
	}

	switch m.state {
	case formView:
		return m.viewForm()
	case spaceSelectionView:
		return appStyle.Render(m.spaceList.View())
	case listSelectionView:
		return appStyle.Render(m.folderlessList.View())
	case createTaskTitleView:
		return m.viewCreateTaskTitle()
	case createTaskDescView:
		return m.viewCreateTaskDesc()
	case createTaskStatusView:
		return appStyle.Render(m.statusList.View())
	case createTaskAssigneeView:
		return appStyle.Render(m.assigneeList.View())
	case createTaskPriorityView:
		return appStyle.Render(m.priorityList.View())
	case taskCreatedView:
		return m.viewTaskCreated()
	case taskDeletedView:
		return m.viewTaskDeleted()
	case listView:
		if m.loading {
			return fmt.Sprintf("\n\n   %s Saving... \n\n", m.spinner.View())
		}
		return appStyle.Render(m.list.View())
	case statusUpdateView:
		return appStyle.Render(m.statusList.View())
	case taskDetailView:
		return m.viewTaskDetail()
	case editTaskView:
		return m.viewEditTask()
	case deleteConfirmationView:
		return m.viewDeleteConfirmation()
	}
	return ""
}

// --- UPDATE & VIEW (FORM) ---
func updateForm(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd = make([]tea.Cmd, len(m.inputs))
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "shift+tab", "enter", "up", "down":
			s := msg.String()
			if s == "enter" && m.focusIndex == len(m.inputs)-1 {
				m.apiToken = m.inputs[0].Value()
				m.teamID = m.inputs[1].Value()
				m.state = spaceSelectionView
				sl := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
				sl.Title = "Select a Space"
				m.spaceList = sl
				return m, tea.Batch(
					saveCredentialsCmd(m.apiToken, m.teamID),
					fetchSpacesCmd(m.apiToken, m.teamID),
				)
			}
			if s == "up" || s == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}
			if m.focusIndex > len(m.inputs) {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs) - 1
			}
			for i := range m.inputs {
				if i == m.focusIndex {
					cmds[i] = m.inputs[i].Focus()
					m.inputs[i].PromptStyle = focusedStyle
				} else {
					m.inputs[i].Blur()
					m.inputs[i].PromptStyle = noStyle
				}
			}
			return m, tea.Batch(cmds...)
		}
	}
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return m, tea.Batch(cmds...)
}

func (m model) viewForm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Enter ClickUp Credentials") + "\n\n")
	for i := range m.inputs {
		b.WriteString(m.inputs[i].View() + "\n")
	}
	b.WriteString(helpStyle.Render("\nenter to submit • tab to navigate • esc to quit"))
	return appStyle.Render(b.String())
}

// --- UPDATE & VIEW (SPACE SELECTION) ---
func updateSpaceSelection(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.spaceList.SetSize(msg.Width-h, msg.Height-v)
	case SpacesResponse:
		items := make([]list.Item, len(msg.Spaces))
		for i, s := range msg.Spaces {
			items[i] = s
		}
		m.spaceList.SetItems(items)
	case error:
		m.err = msg
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "enter" {
			selected, ok := m.spaceList.SelectedItem().(Space)
			if ok {
				m.spaceID = selected.ID
				if m.isCreatingTask {
					m.state = listSelectionView
					ll := list.New([]list.Item{}, list.NewDefaultDelegate(), m.width, m.height)
					ll.Title = "Select a List in " + selected.Name
					m.folderlessList = ll
					return m, tea.Batch(
						fetchFolderlessListsCmd(m.apiToken, m.spaceID),
						fetchFoldersWithListsCmd(m.apiToken, m.spaceID),
					)
				}
				m.state = listView
				h, v := appStyle.GetFrameSize()
				l := list.New([]list.Item{}, list.NewDefaultDelegate(), m.width-h, m.height-v)
				l.Title = "Tasks in " + selected.Name
				l.SetShowStatusBar(true)
				l.SetFilteringEnabled(true)
				l.AdditionalShortHelpKeys = func() []key.Binding {
					return []key.Binding{
						key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
						key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
						key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
					}
				}
				m.list = l
				return m, fetchTasksCmd(m.apiToken, m.teamID, m.spaceID)
			}
		}
	}
	m.spaceList, cmd = m.spaceList.Update(msg)
	return m, cmd
}

// --- UPDATE & VIEW (LIST SELECTION) ---
func updateListSelection(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.folderlessList.SetSize(msg.Width-h, msg.Height-v)
	case ListsResponse:
		for _, l := range msg.Lists {
			m.allLists = append(m.allLists, l)
		}
		m.folderlessList.SetItems(m.allLists)
	case FoldersResponse:
		for _, f := range msg.Folders {
			for _, l := range f.Lists {
				l.Name = fmt.Sprintf("%s / %s", f.Name, l.Name)
				m.allLists = append(m.allLists, l)
			}
		}
		m.folderlessList.SetItems(m.allLists)
	case error:
		m.err = msg
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "enter" {
			selected, ok := m.folderlessList.SelectedItem().(ListInfo)
			if ok {
				m.listID = selected.ID
				m.state = createTaskTitleView
				m.titleInput = textinput.New()
				m.titleInput.Placeholder = "Task Title"
				m.titleInput.Focus()
				return m, nil
			}
		}
	}
	m.folderlessList, cmd = m.folderlessList.Update(msg)
	return m, cmd
}

// --- UPDATE & VIEW (CREATE TASK) ---
func updateCreateTaskTitle(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			m.newTaskTitle = m.titleInput.Value()
			m.state = createTaskDescView
			m.descriptionBox = textarea.New()
			m.descriptionBox.Placeholder = "Task Description"
			m.descriptionBox.Focus()
			return m, nil
		}
	}
	m.titleInput, cmd = m.titleInput.Update(msg)
	return m, cmd
}

func (m model) viewCreateTaskTitle() string {
	return fmt.Sprintf("Enter Task Title:\n\n%s", m.titleInput.View())
}

func updateCreateTaskDesc(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlD {
			m.newTaskDesc = m.descriptionBox.Value()
			m.state = createTaskStatusView
			h, v := appStyle.GetFrameSize()
			m.statusList = list.New([]list.Item{}, statusDelegate{}, m.width-h, m.height-v)
			m.statusList.Title = "Select Status"
			m.statusList.SetShowHelp(false)
			return m, fetchStatusesCmd(m.apiToken, m.spaceID)
		}
	}
	m.descriptionBox, cmd = m.descriptionBox.Update(msg)
	return m, cmd
}

func (m model) viewCreateTaskDesc() string {
	return fmt.Sprintf("Enter Task Description (Ctrl+D to finish):\n\n%s", m.descriptionBox.View())
}

func updateCreateTaskStatus(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.statusList.SetSize(msg.Width-h, msg.Height-v)
	case StatusesResponse:
		items := make([]list.Item, len(msg.Statuses))
		for i, s := range msg.Statuses {
			items[i] = s
		}
		m.statusList.SetItems(items)
	case tea.KeyMsg:
		if msg.String() == "enter" {
			selected, ok := m.statusList.SelectedItem().(Status)
			if ok {
				m.newTaskStatus = selected.Status
				m.state = createTaskAssigneeView
				h, v := appStyle.GetFrameSize()
				m.assigneeList = list.New([]list.Item{}, assigneeDelegate{selected: m.selectedAssignees}, m.width-h, m.height-v)
				m.assigneeList.Title = "Select Assignees (space to select, enter to confirm)"
				return m, fetchAssigneesCmd(m.apiToken, m.listID)
			}
		}
	}
	m.statusList, cmd = m.statusList.Update(msg)
	return m, cmd
}

func updateCreateTaskAssignee(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.assigneeList.SetSize(msg.Width-h, msg.Height-v)
	case MembersResponse:
		items := make([]list.Item, len(msg.Members))
		for i, member := range msg.Members {
			items[i] = member
		}
		m.assigneeList.SetItems(items)
	case tea.KeyMsg:
		switch msg.String() {
		case " ":
			selected, ok := m.assigneeList.SelectedItem().(Member)
			if ok {
				if _, exists := m.selectedAssignees[selected.ID]; exists {
					delete(m.selectedAssignees, selected.ID)
				} else {
					m.selectedAssignees[selected.ID] = struct{}{}
				}
				m.assigneeList.SetDelegate(assigneeDelegate{selected: m.selectedAssignees})
			}
		case "enter":
			for id := range m.selectedAssignees {
				m.newTaskAssignees = append(m.newTaskAssignees, id)
			}
			m.state = createTaskPriorityView
			h, v := appStyle.GetFrameSize()
			m.priorityList = list.New(priorities, priorityDelegate{}, m.width-h, m.height-v)
			m.priorityList.Title = "Select Priority"
			return m, nil
		}
	}
	m.assigneeList, cmd = m.assigneeList.Update(msg)
	return m, cmd
}

func updateCreateTaskPriority(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.priorityList.SetSize(msg.Width-h, msg.Height-v)
	case tea.KeyMsg:
		if msg.String() == "enter" {
			selected, ok := m.priorityList.SelectedItem().(Priority)
			if ok {
				m.newTaskPriority = selected.Value
				return m, createTaskCmd(m.apiToken, m.listID, m.newTaskTitle, m.newTaskDesc, m.newTaskStatus, m.newTaskAssignees, m.newTaskPriority)
			}
		}
	case string:
		if msg == "create_success" {
			m.state = taskCreatedView
			m.progress = progress.New(progress.WithDefaultGradient())
			return m, func() tea.Msg { return tickMsg(time.Now()) }
		}
	}
	m.priorityList, cmd = m.priorityList.Update(msg)
	return m, cmd
}

// --- UPDATE & VIEW (TASK CREATED) ---
func updateTaskCreated(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.progress.Percent() == 1.0 {
			m.quitting = true
			return m, tea.Quit
		}
		cmd := m.progress.IncrPercent(0.25)
		return m, tea.Batch(cmd, func() tea.Msg {
			time.Sleep(time.Millisecond * 200)
			return tickMsg(time.Now())
		})
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m model) viewTaskCreated() string {
	return "\n   Task Created Successfully!\n\n" + m.progress.View() + "\n\n   Quitting..."
}

// --- UPDATE & VIEW (LIST) ---
func updateList(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case spinner.TickMsg:
		if m.loading {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case []list.Item:
		m.loading = false
		m.list.SetItems(msg)
	case error:
		m.err = msg
		return m, tea.Quit
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch keypress := msg.String(); keypress {
		case "d":
			selected, ok := m.list.SelectedItem().(Task)
			if ok {
				m.selectedTask = selected
				m.state = deleteConfirmationView
			}
		case "e":
			selected, ok := m.list.SelectedItem().(Task)
			if ok {
				m.state = editTaskView
				m.insertMode = false
				m.selectedTask = selected
				m.selectedStatus = selected.Status.Status

				m.descriptionBox = textarea.New()
				m.descriptionBox.SetValue(selected.Content)

				m.commentBox = textinput.New()
				m.commentBox.Placeholder = "New comment..."

				m.commandInput = textinput.New()
				m.commandInput.Prompt = ":"
				m.commandInput.CharLimit = 5
				return m, nil
			}
		case "v":
			selected, ok := m.list.SelectedItem().(Task)
			if ok {
				m.state = taskDetailView
				m.selectedTask = Task{}
				m.comments = nil
				m.commentsLoaded = false
				m.viewport = viewport.New(m.width-2, m.height-2)
				m.viewport.SetContent("Loading task details and comments...")
				return m, tea.Batch(
					fetchTaskDetailsCmd(m.apiToken, selected.ID),
					fetchCommentsCmd(m.apiToken, selected.ID),
				)
			}
		}
	case string:
		if msg == "refresh_list_success" {
			m.loading = true
			return m, tea.Batch(
				m.spinner.Tick,
				fetchTasksCmd(m.apiToken, m.teamID, m.spaceID),
			)
		}
	}
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// --- UPDATE & VIEW (STATUS UPDATE) ---
func updateStatusUpdate(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.statusList.SetSize(msg.Width-h, msg.Height-v)
	case StatusesResponse:
		items := make([]list.Item, len(msg.Statuses))
		for i, s := range msg.Statuses {
			items[i] = s
		}
		m.statusList.SetItems(items)
	case error:
		m.err = msg
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = editTaskView
			return m, nil
		case "enter":
			selected, ok := m.statusList.SelectedItem().(Status)
			if ok {
				m.state = editTaskView
				m.selectedStatus = selected.Status
			}
		}
	}
	m.statusList, cmd = m.statusList.Update(msg)
	return m, cmd
}

// --- UPDATE & VIEW (TASK DETAIL) ---
func updateTaskDetail(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width - 2
		m.viewport.Height = msg.Height - 2
	case Task:
		m.selectedTask = msg
	case CommentsResponse:
		m.comments = msg.Comments
		m.commentsLoaded = true
	case error:
		m.err = msg
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.state = listView
			return m, nil
		}
	}
	if m.selectedTask.ID != "" {
		var b strings.Builder
		header := titleStyle.Render(m.selectedTask.Name)
		content := fmt.Sprintf("Status: %s\n\n---\n\n%s", m.selectedTask.Status.Status, m.selectedTask.Content)
		b.WriteString(header)
		b.WriteString("\n")
		b.WriteString(content)
		if m.commentsLoaded {
			b.WriteString("\n\n---\n\n")
			b.WriteString(titleStyle.Render("Comments"))
			b.WriteString("\n\n")
			if len(m.comments) == 0 {
				b.WriteString("No comments on this task.")
			} else {
				for _, comment := range m.comments {
					var commentText strings.Builder
					for _, part := range comment.Comment {
						commentText.WriteString(part.Text)
					}
					ts, _ := time.Parse(time.RFC3339, comment.Date)
					b.WriteString(fmt.Sprintf("From: %s (%s)\n", comment.User.Username, ts.Format("2006-01-02 15:04")))
					b.WriteString(commentText.String())
					b.WriteString("\n\n")
				}
			}
		}
		m.viewport.SetContent(b.String())
	}
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m model) viewTaskDetail() string {
	return appStyle.Render(m.viewport.View())
}

// --- UPDATE & VIEW (EDIT TASK) ---
func updateEditTask(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.commandMode {
			switch msg.Type {
			case tea.KeyEnter:
				command := m.commandInput.Value()
				m.commandInput.Reset()
				m.commandMode = false
				switch command {
				case "w":
					m.state = listView
					var updateCmds []tea.Cmd
					commentCmd := postCommentCmd(m.apiToken, m.selectedTask.ID, m.commentBox.Value())
					if commentCmd != nil {
						updateCmds = append(updateCmds, commentCmd)
					}

					descChanged := m.descriptionBox.Value() != m.selectedTask.Content
					statusChanged := m.selectedStatus != m.selectedTask.Status.Status
					if descChanged || statusChanged {
						var newDesc, newStatus string
						if descChanged {
							newDesc = m.descriptionBox.Value()
						}
						if statusChanged {
							newStatus = m.selectedStatus
						}
						taskCmd := updateTaskCmd(m.apiToken, m.selectedTask.ID, newDesc, newStatus)
						if taskCmd != nil {
							updateCmds = append(updateCmds, taskCmd)
						}
					}
					if len(updateCmds) > 0 {
						return m, tea.Batch(updateCmds...)
					}
					return m, nil
				case "q!":
					m.state = listView
					return m, nil
				case "q":
					m.quitting = true
					return m, tea.Quit
				}
			case tea.KeyEsc:
				m.commandMode = false
				m.commandInput.Reset()
			}
		} else if m.insertMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.insertMode = false
				m.descriptionBox.Blur()
				m.commentBox.Blur()
			case tea.KeyTab:
				if m.descriptionBox.Focused() {
					m.descriptionBox.Blur()
					m.commentBox.Focus()
				} else {
					m.descriptionBox.Focus()
					m.commentBox.Blur()
				}
			}
		} else { // Normal mode
			switch msg.String() {
			case ":":
				m.commandMode = true
				m.commandInput.Focus()
				return m, nil
			case "i":
				m.insertMode = true
				m.descriptionBox.Focus()
			case "a":
				m.insertMode = true
				m.commentBox.Focus()
			case "s":
				m.state = statusUpdateView
				sl := list.New([]list.Item{}, statusDelegate{}, 0, 0)
				sl.Title = "Select new status for: " + m.selectedTask.Name
				sl.SetShowHelp(false)
				m.statusList = sl
				return m, fetchStatusesCmd(m.apiToken, m.selectedTask.Space.ID)
			case "q":
				m.state = listView
				return m, nil
			}
		}
	}

	if m.commandMode {
		m.commandInput, cmd = m.commandInput.Update(msg)
	} else if m.insertMode {
		if m.descriptionBox.Focused() {
			m.descriptionBox, cmd = m.descriptionBox.Update(msg)
		} else {
			m.commentBox, cmd = m.commentBox.Update(msg)
		}
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) viewEditTask() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Editing: " + m.selectedTask.Name))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Status: %s\n\n", m.selectedStatus))
	b.WriteString("Description:\n")
	b.WriteString(m.descriptionBox.View())
	b.WriteString("\n\nAdd Comment:\n")
	b.WriteString(m.commentBox.View())

	if m.commandMode {
		b.WriteString("\n" + m.commandInput.View())
	} else if m.insertMode {
		b.WriteString(helpStyle.Render("\n\n[INSERT MODE] esc to exit • tab to switch"))
	} else {
		b.WriteString(helpStyle.Render("\n\n[NORMAL MODE] i: edit desc • a: add comment • s: change status • q: back to list • :: command"))
	}
	return appStyle.Render(b.String())
}

// --- UPDATE & VIEW (DELETE CONFIRMATION) ---
func updateDeleteConfirmation(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.state = taskDeletedView
			m.progress = progress.New(progress.WithDefaultGradient())
			return m, tea.Batch(
				deleteTaskCmd(m.apiToken, m.selectedTask.ID),
				func() tea.Msg { return tickMsg(time.Now()) },
			)
		case "n", "N", "esc":
			m.state = listView
			return m, nil
		}
	}
	return m, nil
}

func (m model) viewDeleteConfirmation() string {
	return fmt.Sprintf("\n\n   Are you sure you want to delete the task '%s'? (y/n)\n\n", m.selectedTask.Name)
}

func updateTaskDeleted(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case string:
		if msg == "delete_success" {
			// Start the progress bar animation
			return m, func() tea.Msg { return tickMsg(time.Now()) }
		}
	case tickMsg:
		if m.progress.Percent() == 1.0 {
			m.state = listView
			m.loading = true
			return m, fetchTasksCmd(m.apiToken, m.teamID, m.spaceID)
		}
		cmd := m.progress.IncrPercent(0.25)
		return m, tea.Batch(cmd, func() tea.Msg {
			time.Sleep(time.Millisecond * 200)
			return tickMsg(time.Now())
		})
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m model) viewTaskDeleted() string {
	return "\n   Task Deleted Successfully!\n\n" + m.progress.View() + "\n\n   Returning to list..."
}

// --- MAIN (CLI) ---
func loadConfig() {
	home, err := os.UserHomeDir()
	if err == nil {
		_ = godotenv.Load(filepath.Join(home, ".clup.env"))
	}
	_ = godotenv.Load() // Load .env in current directory (overrides home)
}

var rootCmd = &cobra.Command{
	Use:   "clup",
	Short: "A TUI for ClickUp",
	Run: func(cmd *cobra.Command, args []string) {
		loadConfig()
		apiToken := os.Getenv("CLICKUP_API_TOKEN")
		teamID := os.Getenv("CLICKUP_TEAM_ID")
		p := tea.NewProgram(newModel(apiToken, teamID, false), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running program:", err)
			os.Exit(1)
		}
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Find and interact with a specific task",
	Run: func(cmd *cobra.Command, args []string) {
		loadConfig()
		apiToken := os.Getenv("CLICKUP_API_TOKEN")
		teamID := os.Getenv("CLICKUP_TEAM_ID")
		if apiToken == "" || teamID == "" {
			fmt.Println("API token and team ID must be set in your environment or a .env file.")
			os.Exit(1)
		}

		tasksResponse := fetchAllTasksCmd(apiToken, teamID)().(TasksResponse)
		var tasksForFZF []string
		taskMap := make(map[string]Task)
		for _, task := range tasksResponse.Tasks {
			line := fmt.Sprintf("[%s] %s", task.ID, task.Name)
			tasksForFZF = append(tasksForFZF, line)
			taskMap[line] = task
		}

		fzfCmd := exec.Command("fzf")
		fzfIn, _ := fzfCmd.StdinPipe()
		fzfOut, _ := fzfCmd.StdoutPipe()

		go func() {
			defer fzfIn.Close()
			io.WriteString(fzfIn, strings.Join(tasksForFZF, "\n"))
		}()

		err := fzfCmd.Start()
		if err != nil {
			fmt.Println("Error starting fzf:", err)
			os.Exit(1)
		}

		selectedBytes, _ := io.ReadAll(fzfOut)
		err = fzfCmd.Wait()
		if err != nil {
			os.Exit(0)
		}

		selectedLine := strings.TrimSpace(string(selectedBytes))
		if selectedLine == "" {
			fmt.Println("No task selected.")
			os.Exit(0)
		}

		selectedTask := taskMap[selectedLine]

		fmt.Println("Selected:", selectedTask.Name)
		fmt.Print("What do you want to do? (view/edit): ")
		var action string
		fmt.Scanln(&action)

		initialModel := newModel(apiToken, teamID, false)
		initialModel.selectedTask = selectedTask
		initialModel.selectedStatus = selectedTask.Status.Status
		initialModel.spaceID = selectedTask.Space.ID

		initialModel.list = list.New([]list.Item{}, list.NewDefaultDelegate(), 1, 1)

		switch action {
		case "view":
			initialModel.state = taskDetailView
			initialModel.viewport = viewport.New(100, 40)
		case "edit":
			initialModel.state = editTaskView
			initialModel.insertMode = false
			initialModel.descriptionBox = textarea.New()
			initialModel.descriptionBox.SetValue(selectedTask.Content)
			initialModel.commentBox = textinput.New()
			initialModel.commentBox.Placeholder = "New comment..."
			initialModel.commandInput = textinput.New()
			initialModel.commandInput.Prompt = ":"
			initialModel.commandInput.CharLimit = 5
		default:
			fmt.Println("Invalid action.")
			os.Exit(1)
		}

		p := tea.NewProgram(initialModel, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running program:", err)
			os.Exit(1)
		}
	},
}

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Create a new task",
	Run: func(cmd *cobra.Command, args []string) {
		loadConfig()
		apiToken := os.Getenv("CLICKUP_API_TOKEN")
		teamID := os.Getenv("CLICKUP_TEAM_ID")
		p := tea.NewProgram(newModel(apiToken, teamID, true), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running program:", err)
			os.Exit(1)
		}
	},
}

func main() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(taskCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
