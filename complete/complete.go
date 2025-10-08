package complete

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	rw "github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/truncate"
)

// Values is the interface to the values displayed by the completion
// bubble.
type Values interface {
	// NumCategories returns the number of categories to display.
	NumCategories() int

	// CategoryTitle returns the title of a category.
	CategoryTitle(catIdx int) string

	// NumEntries returns the number of entries in a given category.
	NumEntries(catIdx int) int

	// Entry returns the entry in a category.
	Entry(catIdx, entryIdx int) Entry
}

// Entry is the interface to one completion candidate.
type Entry interface {
	// Title is the main displayed text.
	Title() string

	// Description is the explanation for the entry.
	Description() string
}

// Styles contain style definitions for the completions component.
type Styles struct {
	FocusedTitleBar             lipgloss.Style
	FocusedTitle                lipgloss.Style
	BlurredTitleBar             lipgloss.Style
	BlurredTitle                lipgloss.Style
	Item                        lipgloss.Style
	SelectedItem                lipgloss.Style
	Spinner                     lipgloss.Style
	FilterPrompt                lipgloss.Style
	FilterCursor                lipgloss.Style
	PaginationStyle             lipgloss.Style
	DefaultFilterCharacterMatch lipgloss.Style
	ActivePaginationDot         lipgloss.Style
	InactivePaginationDot       lipgloss.Style
	ArabicPagination            lipgloss.Style
	DividerDot                  lipgloss.Style
	PlaceholderDescription      lipgloss.Style
	Description                 lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for the
// completions component.
var DefaultStyles = func() (c Styles) {
	ls := list.DefaultStyles()
	subtle := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}

	// Chalk-inspired colors
	chalkGreen := lipgloss.AdaptiveColor{Light: "#00A651", Dark: "#2aa853"}
	chalkGreenLight := lipgloss.AdaptiveColor{Light: "#9ad2a7", Dark: "#9ad2a7"} // Same as SQL keywords
	chalkGray := lipgloss.AdaptiveColor{Light: "#666666", Dark: "#e2e1ed"}

	c.Item = lipgloss.NewStyle().PaddingLeft(1)
	c.SelectedItem = lipgloss.NewStyle().PaddingLeft(1).Foreground(chalkGreen).Bold(true)

	c.FocusedTitleBar = lipgloss.NewStyle()
	c.BlurredTitleBar = lipgloss.NewStyle()
	c.FocusedTitle = lipgloss.NewStyle().Foreground(chalkGreenLight).Underline(true)
	c.BlurredTitle = c.FocusedTitle.Copy().Foreground(subtle)
	c.Spinner = ls.Spinner
	c.FilterPrompt = ls.FilterPrompt
	c.FilterCursor = ls.FilterCursor
	c.PaginationStyle = lipgloss.NewStyle()
	c.DefaultFilterCharacterMatch = ls.DefaultFilterCharacterMatch
	c.ActivePaginationDot = ls.ActivePaginationDot
	c.InactivePaginationDot = ls.InactivePaginationDot
	c.ArabicPagination = ls.ArabicPagination
	c.DividerDot = lipgloss.NewStyle()
	c.Description = lipgloss.NewStyle().Foreground(chalkGray).PaddingLeft(2)
	c.PlaceholderDescription = lipgloss.NewStyle().Foreground(chalkGray)

	return c
}()

// KeyMap defines keybindings for navigating the completions.
type KeyMap struct {
	list.KeyMap
	NextCompletions  key.Binding
	PrevCompletions  key.Binding
	AcceptCompletion key.Binding
	Abort            key.Binding
}

// DefaultKeyMap is the default set of key bindings.
var DefaultKeyMap = KeyMap{
	KeyMap: list.KeyMap{
		CursorUp:             key.NewBinding(key.WithKeys("up", "ctrl+p", "shift+tab"), key.WithHelp("C-p/↑", "prev entry")),
		CursorDown:           key.NewBinding(key.WithKeys("down", "ctrl+n", "tab"), key.WithHelp("C-n/↓", "next entry")),
		NextPage:             key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "prev page/column")),
		PrevPage:             key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "next page/column")),
		GoToStart:            key.NewBinding(key.WithKeys("ctrl+a", "home"), key.WithHelp("C-a/home", "start of column")),
		GoToEnd:              key.NewBinding(key.WithKeys("ctrl+e", "end"), key.WithHelp("C-e/end", "end of column")),
		Filter:               key.NewBinding(key.WithKeys("/", ""), key.WithHelp("/", "filter")),
		ClearFilter:          key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("C-g", "clear/cancel")),
		CancelWhileFiltering: key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("C-g", "clear/cancel")),
		AcceptWhileFiltering: key.NewBinding(key.WithKeys("enter", "ctrl+j"), key.WithHelp("C-j/enter", "accept filter")),
		ShowFullHelp:         key.NewBinding(key.WithKeys("alt+?"), key.WithHelp("M-?", "toggle key help")),
		CloseFullHelp:        key.NewBinding(key.WithKeys("alt+?"), key.WithHelp("M-?", "toggle key help")),
	},
	NextCompletions:  key.NewBinding(key.WithKeys("right", "alt+n"), key.WithHelp("→/M-n", "next column")),
	PrevCompletions:  key.NewBinding(key.WithKeys("left", "alt+p"), key.WithHelp("←/M-p", "prev column")),
	AcceptCompletion: key.NewBinding(key.WithKeys("enter", "ctrl+j"), key.WithHelp("C-j/enter/tab", "accept")),
	Abort:            key.NewBinding(key.WithKeys("ctrl+c", "esc"), key.WithHelp("C-c/esc", "close/cancel")),
}

// Model is the model that implements the completion
// selector widget.
type Model struct {
	Err error

	// KeyMap is the key bindings for navigating the completions.
	KeyMap KeyMap

	// Styles is the styles to use for display.
	Styles Styles

	// AcceptedValue is the result of the selection.
	AcceptedValue Entry

	width     int
	height    int
	maxHeight int
	focused   bool

	values Values

	selectedList  int
	listItems     [][]list.Item
	valueLists    []*list.Model
	categoryNames []string
}

func (m *Model) Debug() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "width: %d, height: %d, maxHeight: %d\n", m.width, m.height, m.maxHeight)
	fmt.Fprintf(&buf, "num lists: %d\n", len(m.valueLists))
	fmt.Fprintf(&buf, "selectedList: %d\n", m.selectedList)
	if len(m.valueLists) > 0 {
		fmt.Fprintf(&buf, "selected item: %v\n", m.valueLists[m.selectedList].SelectedItem())
	}
	fmt.Fprintf(&buf, "accepted: %+v / err %v\n", m.AcceptedValue, m.Err)
	return buf.String()
}

var _ tea.Model = (*Model)(nil)

func New() Model {
	return Model{
		KeyMap:  DefaultKeyMap,
		Styles:  DefaultStyles,
		focused: true,
	}
}

type candidateItem struct{ Entry }

var _ list.Item = candidateItem{}

// FilterValue implements the list.Item interface.
func (s candidateItem) FilterValue() string {
	e := Entry(s)
	return e.Title() + "\n" + e.Description()
}

func convertToItems(values Values, catIdx int) (res []list.Item, maxWidth int) {
	numE := values.NumEntries(catIdx)
	res = make([]list.Item, numE)
	for i := 0; i < numE; i++ {
		it := values.Entry(catIdx, i)
		maxWidth = max(maxWidth, rw.StringWidth(it.Title()))
		res[i] = candidateItem{it}
	}
	return res, maxWidth
}

type renderer struct {
	m       *Model
	listIdx int
	width   int
}

var _ list.ItemDelegate = (*renderer)(nil)

// Render is part of the list.ItemDelegate interface.
func (r *renderer) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(candidateItem)
	if !ok {
		return
	}
	s := i.Title()
	iw := rw.StringWidth(s)
	if iw < r.width {
		s += strings.Repeat(" ", r.width-iw)
	}
	st := &r.m.Styles
	fn := st.Item.Render
	if r.m.selectedList == r.listIdx && index == m.Index() {
		fn = st.SelectedItem.Render
	}
	fmt.Fprint(w, fn(s))
}

// Height is part of the list.ItemDelegate interface.
func (r *renderer) Height() int {
	return 1
}

// Spacing is part of the list.ItemDelegate interface.
func (r *renderer) Spacing() int { return 0 }

// Update is part of the list.ItemDelegate interface.
func (r *renderer) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// SetWidth changes the width.
func (m *Model) SetWidth(width int) {
	m.width = width
	// Update all list widths to match
	// Ensure minimum width to avoid rendering issues
	minWidth := 10
	listWidth := width
	if listWidth < minWidth {
		listWidth = minWidth
	}
	for _, l := range m.valueLists {
		l.SetWidth(listWidth)
	}
}

// SetHeight changes the height.
func (m *Model) SetHeight(height int) {
	// Make space for the description string.
	m.height = clamp(height, 2, m.maxHeight)
	for _, l := range m.valueLists {
		l.SetHeight(m.height - 1)
		// Ensure paginator shows 5 items per page
		l.Paginator.PerPage = 4
		// Force recomputing the keybindings, which
		// is dependent on the page size.
		l.SetFilteringEnabled(false)
	}
}

// GetHeight retrieves the current height.
func (m *Model) GetHeight() int {
	return m.height
}

// GetHeight retrieves the maximum height.
func (m *Model) GetMaxHeight() int {
	return m.maxHeight
}

// SetValues resets the values. It also recomputes the height.
func (m *Model) SetValues(values Values) {
	m.Err = nil
	m.selectedList = 0
	m.values = values
	numCats := values.NumCategories()
	m.valueLists = make([]*list.Model, numCats)
	m.listItems = make([][]list.Item, numCats)
	m.categoryNames = make([]string, numCats)
	const stdHeight = 10
	listDecorationRows :=
		1 +
			max(
				m.Styles.FocusedTitleBar.GetVerticalPadding(),
				m.Styles.BlurredTitleBar.GetVerticalPadding()) +
			max(
				m.Styles.FocusedTitleBar.GetVerticalMargins(),
				m.Styles.BlurredTitleBar.GetVerticalMargins()) +
			1 +
			m.Styles.PaginationStyle.GetVerticalPadding() +
			m.Styles.PaginationStyle.GetVerticalMargins()
	m.maxHeight = listDecorationRows

	perItemHeight := 1 + max(
		m.Styles.Item.GetVerticalPadding(),
		m.Styles.SelectedItem.GetVerticalPadding())

	for i := 0; i < numCats; i++ {
		category := values.CategoryTitle(i)
		m.categoryNames[i] = category
		var itemsMaxWidth int
		m.listItems[i], itemsMaxWidth = convertToItems(values, i)
		// Ensure minimum width to avoid rendering issues
		if itemsMaxWidth < 10 {
			itemsMaxWidth = 10
		}
		// Limit to 5 items per page
		itemsToShow := min(len(m.listItems[i]), 5)
		m.maxHeight = max(m.maxHeight, itemsToShow*perItemHeight+listDecorationRows)
		r := &renderer{m: m, listIdx: i, width: itemsMaxWidth}
		l := list.New(m.listItems[i], r, itemsMaxWidth, stdHeight)
		l.Title = "" // Don't use list's built-in title to avoid truncation
		l.KeyMap = m.KeyMap.KeyMap
		l.DisableQuitKeybindings()
		l.SetShowHelp(false)
		l.SetShowStatusBar(false)
		// Set the paginator to show all items (up to 5) per page
		l.Paginator.PerPage = 4
		l.Paginator.Type = paginator.Arabic
		m.valueLists[i] = &l
	}

	// Make space for the description.
	m.maxHeight++

	// Propagate the logical heights to all lists.
	m.SetHeight(m.maxHeight)

	wasFocused := m.focused
	m.Blur()
	if wasFocused {
		m.Focus()
	}
}

// MatchesKeys returns true when the completion
// editor can use the given key message.
func (m *Model) MatchesKey(msg tea.KeyMsg) bool {
	if !m.focused || len(m.valueLists) == 0 {
		return false
	}

	curList := m.valueLists[m.selectedList]
	switch {
	case key.Matches(msg,
		m.KeyMap.CursorUp,
		m.KeyMap.CursorDown,
		m.KeyMap.GoToStart,
		m.KeyMap.GoToEnd,
		m.KeyMap.Filter,
		m.KeyMap.ClearFilter,
		m.KeyMap.CancelWhileFiltering,
		m.KeyMap.AcceptWhileFiltering,
		m.KeyMap.PrevCompletions,
		m.KeyMap.NextCompletions,
		m.KeyMap.NextPage,
		m.KeyMap.PrevPage,
		m.KeyMap.Abort):
		return true
	case !curList.SettingFilter() &&
		key.Matches(msg, m.KeyMap.AcceptCompletion):
		return true
	case curList.SettingFilter():
		return true
	}
	return false
}

// Focus places the focus on the completion editor.
func (m *Model) Focus() {
	m.focused = true
	if len(m.valueLists) == 0 {
		return
	}
	l := m.valueLists[m.selectedList]
	l.Styles.Title = m.Styles.FocusedTitle
	l.Styles.TitleBar = m.Styles.FocusedTitleBar
}

// Blur removes the focus from the completion editor.
func (m *Model) Blur() {
	m.focused = false
	for _, l := range m.valueLists {
		l.Styles.Title = m.Styles.BlurredTitle
		l.Styles.TitleBar = m.Styles.BlurredTitleBar
	}
}

func (m *Model) prevCompletions() {
	wasFocused := m.focused
	m.Blur()
	m.selectedList = (m.selectedList + len(m.valueLists) - 1) % len(m.valueLists)
	curList := m.valueLists[m.selectedList]
	curList.Select(0)
	if wasFocused {
		m.Focus()
	}
}

func (m *Model) nextCompletions() {
	wasFocused := m.focused
	m.Blur()
	m.selectedList = (m.selectedList + 1) % len(m.valueLists)
	m.valueLists[m.selectedList].Select(0)
	if wasFocused {
		m.Focus()
	}
}

// Init implements the tea.Model interface.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements the tea.Model interface.
func (m *Model) Update(imsg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.valueLists) == 0 {
		m.Err = io.EOF
		return m, nil
	}

	curList := m.valueLists[m.selectedList]
	switch msg := imsg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Abort):
			m.AcceptedValue = nil
			m.Err = io.EOF
			imsg = nil
		case !curList.SettingFilter():
			switch {
			case key.Matches(msg, m.KeyMap.PrevCompletions):
				m.prevCompletions()
				imsg = nil
			case key.Matches(msg, m.KeyMap.NextCompletions):
				m.nextCompletions()
				imsg = nil
			case key.Matches(msg, m.KeyMap.NextPage):
				if curList.Paginator.Page >= curList.Paginator.TotalPages-1 {
					m.nextCompletions()
					imsg = nil
				}
			case key.Matches(msg, m.KeyMap.PrevPage):
				if curList.Paginator.Page == 0 {
					m.prevCompletions()
					imsg = nil
				}
			case key.Matches(msg, m.KeyMap.AcceptCompletion):
				v := curList.SelectedItem().(candidateItem)
				m.AcceptedValue = v.Entry
				m.Err = io.EOF
				imsg = nil
			}
		}
	}
	if imsg == nil {
		return m, nil
	}
	newModel, cmd := m.valueLists[m.selectedList].Update(imsg)
	// By default, the list blocks the enter key when the
	// filtering prompt is open but there is no filter entered.
	// We don't like this - enter should just accept the current item.
	newModel.KeyMap.AcceptWhileFiltering.SetEnabled(true)
	// Ensure the list maintains proper dimensions after update
	if m.width >= 10 {
		newModel.SetWidth(m.width)
	}
	if m.height >= 2 {
		newModel.SetHeight(m.height - 1)
	}
	m.valueLists[m.selectedList] = &newModel
	return m, cmd
}

// View implements the tea.Model interface.
func (m *Model) View() string {
	// Guard against rendering with invalid dimensions
	if m.width < 10 || m.height < 2 {
		return ""
	}

	contents := make([]string, len(m.valueLists))
	for i, l := range m.valueLists {
		// Render title manually to avoid truncation
		titleStyle := m.Styles.BlurredTitle
		if i == m.selectedList && m.focused {
			titleStyle = m.Styles.FocusedTitle
		}
		title := titleStyle.Render(m.categoryNames[i])
		contents[i] = title + l.View()
	}
	result := lipgloss.JoinHorizontal(lipgloss.Top, contents...)

	curSelected := m.valueLists[m.selectedList].SelectedItem()
	var desc string
	if curSelected == nil {
		desc = m.Styles.PlaceholderDescription.Render("(no entry seleted)")
	} else {
		item := curSelected.(candidateItem)
		desc = item.Description()
		if desc != "" {
			desc = m.Styles.Description.Render(truncate.String(desc, uint(m.width)))
		} else {
			desc = m.Styles.PlaceholderDescription.Render("")
		}
	}

	// Add underline separator and spacing between completions and description
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}).
		Render(strings.Repeat("─", m.width-4)) // -4 to account for border and padding

	// Combine completions, separator, spacing, and description
	combined := result + separator + "\n" + desc

	// Add border around everything
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}).
		Padding(0, 1).
		Width(m.width)

	return boxStyle.Render(combined)
}

// ShortHelp is part of the help.KeyMap interface.
func (m *Model) ShortHelp() []key.Binding {
	if len(m.valueLists) == 0 {
		return nil
	}

	kb := []key.Binding{
		m.KeyMap.Abort,
	}

	curList := m.valueLists[m.selectedList]
	if !curList.SettingFilter() {
		kb = append(kb,
			m.KeyMap.NextCompletions,
			m.KeyMap.AcceptCompletion,
		)
	}
	return append(kb, curList.ShortHelp()...)
}

// FullHelp is part of the help.KeyMap interface.
func (m *Model) FullHelp() [][]key.Binding {
	if len(m.valueLists) == 0 {
		return nil
	}
	curList := m.valueLists[m.selectedList]
	kb := [][]key.Binding{{
		m.KeyMap.NextCompletions,
		m.KeyMap.PrevCompletions,
		m.KeyMap.AcceptCompletion,
		m.KeyMap.Abort,
	}}
	kb = append(kb, curList.FullHelp()...)
	return kb
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}
