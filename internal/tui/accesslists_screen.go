package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shiva/nginx-manager/internal/accesslist"
)

type accessListsScreen struct {
	deps  Deps
	lists []accesslist.AccessList
	menu  *simpleMenu
	err   string
}

func newAccessListsScreen(deps Deps) *accessListsScreen {
	s := &accessListsScreen{deps: deps}
	s.reload()
	return s
}

func (s *accessListsScreen) reload() {
	lists, err := s.deps.AccessLists.List()
	if err != nil {
		s.err = err.Error()
		return
	}
	s.lists = lists
	items := make([]menuItem, 0, len(lists)+1)
	items = append(items, menuItem{title: "+ Create Access List"})
	for _, l := range lists {
		items = append(items, menuItem{title: l.Name, desc: fmt.Sprintf("%d user(s)", len(l.Users))})
	}
	s.menu = newSimpleMenu(items)
}

func (s *accessListsScreen) Init() tea.Cmd { return nil }

func (s *accessListsScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			if s.menu.Selected() == 0 {
				return s, navPush(newCreateAccessListWizard(s.deps))
			}
			l := s.lists[s.menu.Selected()-1]
			return s, navPush(newAccessListDetailScreen(s.deps, l.Name))
		}
	}
	return s, nil
}

func (s *accessListsScreen) View(width, height int) string {
	body := headerStyle.Render("Access Lists") + "\n"
	if s.err != "" {
		body += errorBoxStyle.Render(s.err)
	} else {
		body += s.menu.View()
	}
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "NGINX MANAGER", body, help)
}

func newCreateAccessListWizard(deps Deps) *wizardScreen {
	fields := []wizardField{
		{Key: "name", Label: "Access list name (e.g. admin-panel):", Kind: fieldText,
			Validate: func(v string) error {
				if v == "" {
					return fmt.Errorf("name must not be empty")
				}
				if deps.AccessLists.Exists(v) {
					return fmt.Errorf("an access list named %q already exists", v)
				}
				return nil
			}},
	}
	return newWizard("Create Access List", fields, func(v map[string]string) (screen, tea.Cmd) {
		if err := deps.AccessLists.Create(v["name"]); err != nil {
			return newResultScreen(deps, "Create Access List", false, err.Error(), newAccessListsScreen(deps)), nil
		}
		return newResultScreen(deps, "Create Access List", true, "Access list created. Add users from its detail screen.", newAccessListDetailScreen(deps, v["name"])), nil
	}, func() (screen, tea.Cmd) { return newAccessListsScreen(deps), nil })
}

// --- Access list detail ---

type accessListDetailScreen struct {
	deps Deps
	name string
	al   accesslist.AccessList
	menu *simpleMenu
}

const (
	aldAddUser = iota
	aldRemoveUser
	aldChangePassword
	aldViewHosts
	aldDelete
)

func newAccessListDetailScreen(deps Deps, name string) *accessListDetailScreen {
	s := &accessListDetailScreen{deps: deps, name: name}
	s.reload()
	return s
}

func (s *accessListDetailScreen) reload() {
	s.al, _ = s.deps.AccessLists.Get(s.name)
	s.menu = newSimpleMenu([]menuItem{
		{title: "Add User"},
		{title: "Remove User"},
		{title: "Change Password"},
		{title: "View Hosts Using This List"},
		{title: "Delete Access List"},
	})
}

func (s *accessListDetailScreen) Init() tea.Cmd { return nil }

func (s *accessListDetailScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if s.menu.HandleKey(km) {
			return s, nil
		}
		switch km.String() {
		case "esc", "q":
			return s, navPop()
		case "enter":
			return s.dispatch(s.menu.Selected())
		}
	}
	return s, nil
}

func (s *accessListDetailScreen) dispatch(choice int) (screen, tea.Cmd) {
	switch choice {
	case aldAddUser:
		return s, navPush(newAddUserWizard(s.deps, s.name))
	case aldRemoveUser:
		return s, navPush(newRemoveUserWizard(s.deps, s.name))
	case aldChangePassword:
		return s, navPush(newChangePasswordWizard(s.deps, s.name))
	case aldViewHosts:
		return s.viewHosts()
	case aldDelete:
		return newConfirmScreen("Delete Access List",
			fmt.Sprintf("Delete access list %q? Hosts referencing it will keep pointing at a missing htpasswd file until updated.", s.name),
			func() (screen, tea.Cmd) {
				if err := s.deps.AccessLists.Delete(s.name); err != nil {
					return newResultScreen(s.deps, "Delete Access List", false, err.Error(), s), nil
				}
				return newResultScreen(s.deps, "Delete Access List", true, "Access list deleted.", newAccessListsScreen(s.deps)), nil
			},
			func() (screen, tea.Cmd) { return s, nil },
		), nil
	}
	return s, nil
}

func (s *accessListDetailScreen) viewHosts() (screen, tea.Cmd) {
	hostList, err := s.deps.Manager.ListHosts()
	if err != nil {
		return newResultScreen(s.deps, "View Hosts", false, err.Error(), s), nil
	}
	content := ""
	found := false
	for _, h := range hostList {
		if h.AccessListName == s.name {
			content += "- " + h.ServerName + " (host level)\n"
			found = true
			continue
		}
		for _, loc := range h.Locations {
			if loc.AccessListName == s.name {
				content += fmt.Sprintf("- %s (location %s)\n", h.ServerName, loc.Path)
				found = true
			}
		}
	}
	if !found {
		content = "No hosts currently use this access list."
	}
	return newViewerScreen("Hosts using "+s.name, content, s), nil
}

func (s *accessListDetailScreen) View(width, height int) string {
	body := headerStyle.Render("Access List: "+s.name) + "\n\n"
	body += "Users: "
	if len(s.al.Users) == 0 {
		body += mutedStyle.Render("(none)")
	} else {
		for i, u := range s.al.Users {
			if i > 0 {
				body += ", "
			}
			body += u
		}
	}
	body += "\n\n" + s.menu.View()
	help := [][2]string{{"↑↓", "Navigate"}, {"Enter", "Select"}, {"Esc", "Back"}}
	return renderFrame(width, height, "NGINX MANAGER", body, help)
}

func newAddUserWizard(deps Deps, listName string) *wizardScreen {
	fields := []wizardField{
		{Key: "username", Label: "Username:", Kind: fieldText, Validate: requireNonEmpty("username")},
		{Key: "password", Label: "Password:", Kind: fieldText, Validate: func(v string) error {
			if len(v) < 4 {
				return fmt.Errorf("password should be at least 4 characters")
			}
			return nil
		}},
	}
	return newWizard("Add User — "+listName, fields, func(v map[string]string) (screen, tea.Cmd) {
		if err := deps.AccessLists.AddUser(listName, v["username"], v["password"]); err != nil {
			return newResultScreen(deps, "Add User", false, err.Error(), newAccessListDetailScreen(deps, listName)), nil
		}
		return newResultScreen(deps, "Add User", true, "User added.", newAccessListDetailScreen(deps, listName)), nil
	}, func() (screen, tea.Cmd) { return newAccessListDetailScreen(deps, listName), nil })
}

func newRemoveUserWizard(deps Deps, listName string) *wizardScreen {
	al, _ := deps.AccessLists.Get(listName)
	options := al.Users
	if len(options) == 0 {
		options = []string{"(no users)"}
	}
	fields := []wizardField{
		{Key: "username", Label: "Remove which user?", Kind: fieldChoice, Options: options},
	}
	return newWizard("Remove User — "+listName, fields, func(v map[string]string) (screen, tea.Cmd) {
		if len(al.Users) == 0 {
			return newAccessListDetailScreen(deps, listName), nil
		}
		if err := deps.AccessLists.RemoveUser(listName, v["username"]); err != nil {
			return newResultScreen(deps, "Remove User", false, err.Error(), newAccessListDetailScreen(deps, listName)), nil
		}
		return newResultScreen(deps, "Remove User", true, "User removed.", newAccessListDetailScreen(deps, listName)), nil
	}, func() (screen, tea.Cmd) { return newAccessListDetailScreen(deps, listName), nil })
}

func newChangePasswordWizard(deps Deps, listName string) *wizardScreen {
	al, _ := deps.AccessLists.Get(listName)
	options := al.Users
	if len(options) == 0 {
		options = []string{"(no users)"}
	}
	fields := []wizardField{
		{Key: "username", Label: "Change password for which user?", Kind: fieldChoice, Options: options},
		{Key: "password", Label: "New password:", Kind: fieldText, Validate: func(v string) error {
			if len(v) < 4 {
				return fmt.Errorf("password should be at least 4 characters")
			}
			return nil
		}},
	}
	return newWizard("Change Password — "+listName, fields, func(v map[string]string) (screen, tea.Cmd) {
		if len(al.Users) == 0 {
			return newAccessListDetailScreen(deps, listName), nil
		}
		if err := deps.AccessLists.SetPassword(listName, v["username"], v["password"]); err != nil {
			return newResultScreen(deps, "Change Password", false, err.Error(), newAccessListDetailScreen(deps, listName)), nil
		}
		return newResultScreen(deps, "Change Password", true, "Password updated.", newAccessListDetailScreen(deps, listName)), nil
	}, func() (screen, tea.Cmd) { return newAccessListDetailScreen(deps, listName), nil })
}
