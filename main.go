package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// ---- НОВОЕ: структура темы (TOML) ----
//
// Файл темы: ~/.config/myapp/theme.toml
//
// Пример (минимальный):
//
// [ui]
// background = "#0f1117"
// foreground = "#c9d1d9"
// accent = "#58a6ff"
// cursor = "#ffcc00"
// selection_bg = "#223244"
//
// [ui.left_panel]
// fg = "#c9d1d9"
// bg = "#111217"
// selected_fg = "#0f1724"
// selected_bg = "#58a6ff"
// selected_bold = true
//
// [ui.right_panel]
// fg = "#c9d1d9"
// bg = "#0f1117"
//
// [ui.statusbar]
// fg = "#9aa4b2"
// bg = "#0b1220"
//
// [markdown.h1]
// fg = "#ff7ab6"
// bold = true
//
// [markdown.inline_code]
// fg = "#111827"
// bg = "#f8fafc"
//
// …и т.д.
//
// Полная схема реализована в типах ниже.

// StyleSpec описывает стиль для одного элемента
type StyleSpec struct {
	FG        string `toml:"fg" json:"fg"`
	BG        string `toml:"bg" json:"bg"`
	Bold      bool   `toml:"bold" json:"bold"`
	Italic    bool   `toml:"italic" json:"italic"`
	Underline bool   `toml:"underline" json:"underline"`
	Reverse   bool   `toml:"reverse" json:"reverse"`
}

// PanelStyle — отдельный тип для панелей, с опциями для выделения
type PanelStyle struct {
	FG            string `toml:"fg"`
	BG            string `toml:"bg"`
	SelectedFG    string `toml:"selected_fg"`
	SelectedBG    string `toml:"selected_bg"`
	SelectedBold  bool   `toml:"selected_bold"`
	DirFG         string `toml:"dir_fg"`
	SelectedDirFG string `toml:"selected_dir_fg"`
}

// UITheme — общие цвета приложения
type UITheme struct {
	Background  string     `toml:"background"`
	Foreground  string     `toml:"foreground"`
	Accent      string     `toml:"accent"`
	Cursor      string     `toml:"cursor"`
	SelectionBG string     `toml:"selection_bg"`
	LeftPanel   PanelStyle `toml:"left_panel"`
	RightPanel  PanelStyle `toml:"right_panel"`
	Statusbar   StyleSpec  `toml:"statusbar"`
}

// MarkdownTheme — стили Markdown
type MarkdownTheme struct {
	H1         StyleSpec `toml:"h1"`
	H2         StyleSpec `toml:"h2"`
	H3         StyleSpec `toml:"h3"`
	InlineCode StyleSpec `toml:"inline_code"`
	CodeBlock  StyleSpec `toml:"codeblock"`
	Link       StyleSpec `toml:"link"`
	ListMarker StyleSpec `toml:"list_marker"`
	Blockquote StyleSpec `toml:"blockquote"`
	Table      struct {
		Header StyleSpec `toml:"header"`
		Border string    `toml:"border"`
	} `toml:"table"`
	HR StyleSpec `toml:"hr"`
}

// Theme — корневая структура
type Theme struct {
	UI       UITheme       `toml:"ui"`
	Markdown MarkdownTheme `toml:"markdown"`
}

// дефолтная тема (fallback)
var defaultTheme = Theme{
	UI: UITheme{
		Background:  "#0f1117",
		Foreground:  "#c9d1d9",
		Accent:      "#88d4ab",
		Cursor:      "#ffcc00",
		SelectionBG: "#223244",
		LeftPanel: PanelStyle{
			FG:           "#444444",
			BG:           "#111217",
			SelectedFG:   "#111111",
			SelectedBG:   "#999999",
			SelectedBold: true,
		},
		RightPanel: PanelStyle{
			FG: "#c9d1d9",
			BG: "#0f1117",
		},
		Statusbar: StyleSpec{
			FG: "#9aa4b2",
			BG: "#0b1220",
		},
	},
	Markdown: MarkdownTheme{
		H1: StyleSpec{FG: "#ff7ab6", Bold: false},
		H2: StyleSpec{FG: "#ff9f43"},
		H3: StyleSpec{FG: "#ffd166", Bold: true},
		InlineCode: StyleSpec{
			FG: "#111827", BG: "#333234",
		},
		CodeBlock: StyleSpec{
			FG: "#ff9999", BG: "#333234",
		},
		Link:       StyleSpec{FG: "#58a6ff", Underline: true},
		ListMarker: StyleSpec{FG: "#9aa4b2", Bold: true},
		Blockquote: StyleSpec{FG: "#94a3b8", Italic: true},
		Table: struct {
			Header StyleSpec `toml:"header"`
			Border string    `toml:"border"`
		}{
			Header: StyleSpec{FG: "#e6edf3"},
			Border: "#3b4252",
		},
		HR: StyleSpec{FG: "#3b4252"},
	},
}

// ---- конец темы ----

// старые цветовые константы — оставлены как запасной вариант
const (
	ColorGrey      = tcell.ColorGrey
	ColorRed       = tcell.ColorRed
	ColorYellow    = tcell.ColorYellow
	ColorWhite     = tcell.ColorWhite
	ColorAqua      = tcell.ColorAqua
	ColorFuchsia   = tcell.ColorFuchsia
	ColorGray      = tcell.ColorGray
	ColorOlive     = tcell.ColorOlive
	ColorLightBlue = tcell.ColorLightBlue
	ColorLightGrey = tcell.ColorLightGrey
	ColorBlack     = tcell.ColorBlack
)

var (
	ColorBlue  = tcell.NewRGBColor(196, 110, 81)
	ColorGreen = tcell.NewRGBColor(93, 93, 93)
)

// Структура для хранения информации о файле
type fileItem struct {
	name  string
	path  string
	isDir bool
}

const textEditorPadding = 2 // Отступ от левой границы текстового редактора

// Основная структура приложения
type App struct {
	screen       tcell.Screen
	currentDir   string
	files        []fileItem
	cursor       int
	showHidden   bool
	showTerminal bool

	currentFile  string
	fileContent  string
	fileModified bool   // флаг, указывающий, был ли файл изменен
	mode         string // "edit" или "preview"
	activePanel  string // "left" или "right"

	// Размеры экрана
	width, height int

	// Позиции курсора в редакторе (в rune-единицах)
	editX, editY int

	// Смещение для прокрутки (в rune-единицах)
	scrollX, scrollY int

	// Размеры панелей
	leftWidth int

	// тема и мьютекс для безопасного доступа
	theme   *Theme
	themeMu sync.RWMutex

	// watcher для темы
	themeWatcher *fsnotify.Watcher
}

// Тип токена для подсветки (остался если понадобится)
type hlToken struct {
	text  string
	style tcell.Style
}

// Получить путь к файлу темы: ~/.config/myapp/theme.toml
func themePath() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		userPath := filepath.Join(home, ".config", "myapp", "theme.toml")
		if _, err := os.Stat(userPath); err == nil {
			return userPath
		}
	}
	// fallback на файл рядом с бинарником
	return "./theme.toml"
}

// ---- Парсинг цвета (hex + числа + имена) ----
func parseColor(s string) tcell.Color {
	s = strings.TrimSpace(s)
	if s == "" {
		return tcell.ColorDefault
	}
	lower := strings.ToLower(s)
	if lower == "default" || lower == "terminal" || lower == "none" || lower == "transparent" {
		return tcell.ColorDefault
	}

	// hex #RRGGBB или #RGB
	if strings.HasPrefix(s, "#") {
		hex := strings.TrimPrefix(s, "#")
		if len(hex) == 3 {
			// expand "abc" -> "aabbcc"
			expanded := make([]byte, 6)
			for i := 0; i < 3; i++ {
				expanded[i*2] = hex[i]
				expanded[i*2+1] = hex[i]
			}
			hex = string(expanded)
		}
		if len(hex) == 6 {
			if v, err := strconv.ParseUint(hex, 16, 32); err == nil {
				r := int32((v >> 16) & 0xFF)
				g := int32((v >> 8) & 0xFF)
				b := int32(v & 0xFF)
				return tcell.NewRGBColor(r, g, b)
			}
		}
	}

	// числовой код (0..255)
	if n, err := strconv.Atoi(s); err == nil {
		return tcell.Color(n)
	}

	// имена
	switch lower {
	case "black":
		return tcell.ColorBlack
	case "red":
		return tcell.ColorRed
	case "green":
		return tcell.ColorGreen
	case "yellow":
		return tcell.ColorYellow
	case "blue":
		return tcell.ColorBlue
	case "magenta", "purple":
		return tcell.ColorPurple
	case "cyan", "teal":
		return tcell.ColorTeal
	case "white":
		return tcell.ColorWhite
	case "gray", "grey":
		return tcell.ColorGrey
	}

	return tcell.ColorDefault

}

// Преобразование StyleSpec -> tcell.Style (с использованием UI-фоллбеков)
func styleFromSpec(spec StyleSpec, ui UITheme) tcell.Style {
	style := tcell.StyleDefault

	// fg
	fg := spec.FG
	if fg == "" {
		fg = ui.Foreground
	}
	if fg != "" {
		style = style.Foreground(parseColor(fg))
	}

	// bg
	bg := spec.BG
	if bg == "" {
		bg = ui.Background
	}
	if bg != "" {
		style = style.Background(parseColor(bg))
	}

	if spec.Bold {
		style = style.Bold(true)
	}
	if spec.Italic {
		style = style.Italic(true)
	}
	if spec.Underline {
		style = style.Underline(true)
	}
	if spec.Reverse {
		style = style.Reverse(true)
	}
	return style

}

// ---- Загрузка и применение темы ----
func loadThemeFromFile(path string) (*Theme, error) {
	var t Theme

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("theme not found at %s", path)
		}
		return nil, fmt.Errorf("cannot access theme file: %v", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("theme path %s is a directory, not a file", path)
	}

	if _, err := toml.DecodeFile(path, &t); err != nil {
		return nil, fmt.Errorf("failed to parse theme: %v", err)
	}

	return &t, nil
}

func (a *App) applyTheme(t *Theme) {
	a.themeMu.Lock()
	defer a.themeMu.Unlock()
	if t == nil {
		// если nil — используем дефолтную
		a.theme = &defaultTheme
	} else {
		a.theme = t
	}
}

// загрузка темы: если нет файла — дефолт
func (a *App) loadTheme() {
	path := themePath()
	fmt.Println("[debug] trying to load theme:", path)
	t, err := loadThemeFromFile(path)
	if err != nil {
		fmt.Println("[debug] theme load failed:", err)
		a.applyTheme(&defaultTheme)
		return
	}
	fmt.Println("[debug] theme loaded successfully!")
	a.applyTheme(t)
}

// Релоад темы (вызов из хоткея)
func (a *App) reloadTheme() {
	// пробуем загрузить; если ошибка — не крашим приложение, оставляем старую тему
	path := themePath()
	t, err := loadThemeFromFile(path)
	if err != nil {
		// можно вывести уведомление — пока просто вернёмся к дефолту
		a.applyTheme(&defaultTheme)
	} else {
		a.applyTheme(t)
	}
	// попросим tcell перерисовать экран
	if a.screen != nil {
		a.screen.Sync()
	}
}

// Наблюдатель за файлом темы (fsnotify). Работает в отдельной горутине.
// Смотрим за директорией, где лежит файл, т.к. иногда файл перезаписывают через tmp-файл.
func (a *App) watchThemeFile() error {
	path := themePath()
	dir := filepath.Dir(path)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	a.themeWatcher = w

	// Добавляем watcher на директорию
	if err := w.Add(dir); err != nil {
		// если не удалось — всё равно продолжаем (приложение будет работать без hot-reload)
		go func() {
			// закрываем watcher через некоторое время, чтобы не утекал
			time.Sleep(10 * time.Millisecond)
			_ = w.Close()
		}()
		return err
	}

	go func() {
		defer w.Close()
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				// интересуют изменения конкретного файла
				if filepath.Clean(ev.Name) == filepath.Clean(path) {
					// WRITE, CREATE, REMOVE, RENAME — в любом случае пробуем перезагрузить тему
					if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
						a.reloadTheme()
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				_ = err
			}
		}
	}()

	return nil

}

// ---- Инициализация приложения (NewApp) ----
func NewApp() (*App, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}

	if err := screen.Init(); err != nil {
		return nil, err
	}

	app := &App{
		screen:       screen,
		currentDir:   "",
		files:        []fileItem{},
		cursor:       0,
		showHidden:   false,
		currentFile:  "",
		fileContent:  "",
		fileModified: false,
		mode:         "edit",
		activePanel:  "left",
		editX:        0,
		editY:        0,
		scrollX:      0,
		scrollY:      0,
		leftWidth:    30,
		theme:        &defaultTheme,
	}

	// Получаем текущую директорию
	if cwd, err := os.Getwd(); err == nil {
		app.currentDir = cwd
	}

	// Загружаем тему (если есть)
	app.loadTheme()
	// пытаемся включить watch (если не удастся — приложение всё равно рабочее)
	_ = app.watchThemeFile()

	app.loadFiles()
	return app, nil

}

// Загрузка файлов из текущей директории
func (a *App) loadFiles() {
	a.files = []fileItem{}

	// Читаем содержимое директории
	entries, err := os.ReadDir(a.currentDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		// Пропускаем скрытые файлы если не включен их показ
		if !a.showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		a.files = append(a.files, fileItem{
			name:  entry.Name(),
			path:  filepath.Join(a.currentDir, entry.Name()),
			isDir: entry.IsDir(),
		})
	}

}

// Открытие выбранного файла или директории
func (a *App) openSelected() {
	if len(a.files) == 0 || a.cursor < 0 || a.cursor >= len(a.files) {
		return
	}

	file := a.files[a.cursor]

	if file.isDir {
		// Переходим в директорию
		a.currentDir = file.path
		a.cursor = 0
		a.loadFiles()
		a.activePanel = "left"
	} else {
		// Открываем файл
		a.openFile(file.path)
		a.activePanel = "right"
	}

}

// Открытие файла для редактирования/предпросмотра
func (a *App) openFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		a.fileContent = fmt.Sprintf("Ошибка чтения файла: %v", err)
		return
	}

	a.currentFile = path
	a.fileContent = string(content)
	a.fileModified = false // сбрасываем флаг изменений при открытии файла
	a.editX = 0
	a.editY = 0
	a.scrollX = 0
	a.scrollY = 0
	a.clampCursor()

	// Если markdown - открываем в режиме preview по умолчанию
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".markdown") {
		a.mode = "preview"
	} else {
		a.mode = "edit"
	}

}

// Удаление выбранного файла
func (a *App) deleteFile() {
	// Проверяем, что файл выбран и мы в левой панели
	if a.activePanel != "left" || len(a.files) == 0 || a.cursor < 0 || a.cursor >= len(a.files) {
		return
	}

	file := a.files[a.cursor]

	// Не удаляем директории (для безопасности)
	if file.isDir {
		return
	}

	// Удаляем файл из файловой системы
	err := os.Remove(file.path)
	if err != nil {
		// Можно добавить уведомление об ошибке
		return
	}

	// Обновляем список файлов
	a.loadFiles()

	// Корректируем позицию курсора, если нужно
	if a.cursor >= len(a.files) && len(a.files) > 0 {
		a.cursor = len(a.files) - 1
	}

}

// Сохранение текущего файла
func (a *App) saveFile() {
	if a.currentFile == "" {
		// Нельзя сохранить файл без имени
		return
	}

	err := os.WriteFile(a.currentFile, []byte(a.fileContent), 0644)
	if err != nil {
		// Можно добавить уведомление об ошибке
		return
	}

	// Сбрасываем флаг изменений после успешного сохранения
	a.fileModified = false

	// Перерисовываем интерфейс, чтобы обновить индикатор изменений
	a.draw()

}

// Переключение активной панели
func (a *App) setActivePanel(panel string) {
	a.activePanel = panel
}

// Переключение между режимами редактирования и предпросмотра
func (a *App) toggleMode() {
	if a.mode == "edit" {
		a.mode = "preview"
	} else {
		a.mode = "edit"
	}
}
func (a *App) toggleTerminal() {
	a.showTerminal = !a.showTerminal
	a.ensureCursorVisible()
}

// Возврат в родительскую директорию
func (a *App) goBack() {
	parent := filepath.Dir(a.currentDir)
	if parent != a.currentDir {
		a.currentDir = parent
		a.cursor = 0
		a.loadFiles()
	}
}

// Переключение показа скрытых файлов
func (a *App) toggleHidden() {
	a.showHidden = !a.showHidden
	a.loadFiles()
}

// Показ справки
func (a *App) showHelp() {
	// Простая реализация справки
	helpText := `Справка по горячим клавишам:


НАВИГАЦИЯ:
↑/↓ - перемещение по списку файлов
→ - открыть файл/папку
← - вернуться в родительскую папку
Enter - открыть выбранный элемент


ПЕРЕКЛЮЧЕНИЕ ПАНЕЛЕЙ:
Ctrl+← - переключить на левую панель
Ctrl+→ - переключить на правую панель


РЕДАКТИРОВАНИЕ:
Tab - переключить режим редактирования/предпросмотра
Ctrl+S - сохранить файл
Delete - удалить файл (в левой панели)


ПРОЧЕЕ:
. - показать/скрыть скрытые файлы
? - показать справку
Ctrl+Q - выйти
Ctrl+R - перезагрузить тему


ИНДИКАТОРЫ:


в заголовке редактора означает, что файл был изменен, но еще не сохранен

ПРЕДПРОСМОТР:
Файлы .md/.markdown открываются по умолчанию в режиме Preview (Tab переключает режим)


Нажмите любую клавишу для закрытия справки…`

	// Временно заменяем содержимое на справку
	oldContent := a.fileContent
	oldActive := a.activePanel
	a.fileContent = helpText
	a.activePanel = "right"
	a.draw()

	// Ждем нажатия клавиши
	for {
		ev := a.screen.PollEvent()
		if _, ok := ev.(*tcell.EventKey); ok {
			break
		}
	}

	// Восстанавливаем содержимое
	a.fileContent = oldContent
	a.activePanel = oldActive
	a.draw()

}

// Получить строки (гарантированно хотя бы одна)
func (a *App) getLines() []string {
	if a.fileContent == "" {
		return []string{""}
	}
	lines := strings.Split(a.fileContent, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// Проверить, является ли файл Markdown файлом
func (a *App) isMarkdownFile() bool {
	low := strings.ToLower(a.currentFile)
	return strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".markdown")
}

// Получить последнее слово в строке
func (a *App) getLastWord(line string) string {
	// Удаляем пробелы в конце строки
	trimmed := strings.TrimRight(line, " \t")

	// Находим начало последнего слова (ищем любой символ, не являющийся буквой, цифрой или подчеркиванием)
	end := len(trimmed)
	for i := len(trimmed) - 1; i >= 0; i-- {
		r := rune(trimmed[i])
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		} else {
			// Нашли разделитель, возвращаем часть строки после него
			return trimmed[i+1 : end]
		}
	}

	// Вся строка состоит из одного слова
	return trimmed

}

// Проверить, является ли символ разделителем
func (a *App) isWordSeparator(r rune) bool {
	return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
}

// Получить текущий уровень отступа строки
func (a *App) getCurrentIndentLevel(line string) int {
	indent := 0
	for _, r := range line {
		if r == '\t' {
			indent++
		} else if r == ' ' {
			// Считаем 4 пробела как 1 табуляцию
			indent++
			// Пропускаем еще 3 пробела
			// Но для этого нужно модифицировать логику
		} else {
			break
		}
	}
	return indent
}

// Создать строку отступа заданного уровня
func (a *App) createIndentString(level int) string {
	// Используем табуляции для отступов
	return strings.Repeat("\t", level)
}

// Установить строки обратно в fileContent
func (a *App) setLines(lines []string) {
	a.fileContent = strings.Join(lines, "\n")
	a.fileModified = true
}

// Ограничить позицию курсора в пределах содержимого
func (a *App) clampCursor() {
	lines := a.getLines()
	if a.editY < 0 {
		a.editY = 0
	}
	if a.editY >= len(lines) {
		a.editY = len(lines) - 1
	}
	lineRunes := []rune(lines[a.editY])
	if a.editX < 0 {
		a.editX = 0
	}
	if a.editX > len(lineRunes) {
		a.editX = len(lineRunes)
	}
}

// helper: display column (in cells) of rune index (sum widths of runes[0:upto])
func runesDisplayWidth(runes []rune, upto int) int {
	if upto <= 0 {
		return 0
	}
	if upto > len(runes) {
		upto = len(runes)
	}
	w := 0
	for i := 0; i < upto; i++ {
		w += runewidth.RuneWidth(runes[i])
	}
	return w
}

// Обеспечить видимость курсора (корректирует scrollX/Y)
func (a *App) ensureCursorVisible() {
	a.width, a.height = a.screen.Size()
	editorWidth := a.width - a.leftWidth - 2 - textEditorPadding
	editorHeight := a.height - 5
	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

	// вертикальная прокрутка (в строках)
	if a.editY < a.scrollY {
		a.scrollY = a.editY
	} else if a.editY >= a.scrollY+editorHeight {
		a.scrollY = a.editY - editorHeight + 1
	}

	// горизонтальная прокрутка: нужно учитывать реальную ширину рун в текущей строке
	lines := a.getLines()
	if a.editY < 0 || a.editY >= len(lines) {
		// защита
		if a.scrollX < 0 {
			a.scrollX = 0
		}
		return
	}
	line := lines[a.editY]
	runes := []rune(line)

	// текущее отображаемое смещение в колонках (cells)
	cursorDisp := runesDisplayWidth(runes, a.editX)
	scrollDisp := runesDisplayWidth(runes, a.scrollX)

	if cursorDisp < scrollDisp {
		// смещаем scrollX в rune-индекс равный editX
		a.scrollX = a.editX
	} else if cursorDisp >= scrollDisp+editorWidth {
		// нужно подобрать новое scrollX (rune-индекс) так, чтобы курсор поместится
		// минимально уменьшаем scrollX
		newScroll := a.editX
		// двигаемся назад, пока отображаемая ширина от newScroll до editX больше нужной
		for newScroll > 0 {
			if runesDisplayWidth(runes, newScroll) <= cursorDisp-editorWidth+1 {
				break
			}
			newScroll--
		}
		a.scrollX = newScroll
	}

	if a.scrollY < 0 {
		a.scrollY = 0
	}
	if a.scrollX < 0 {
		a.scrollX = 0
	}

}

// Получить текущую тему (копия указателя под RLock)
func (a *App) getTheme() *Theme {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	return a.theme
}

// Отрисовка интерфейса
func (a *App) draw() {
	a.screen.Clear()

	// Получаем размеры экрана
	a.width, a.height = a.screen.Size()

	// Рисуем левую панель (файловый менеджер)
	a.drawFileList()

	// Рисуем правую панель (редактор/предпросмотр)
	a.drawEditor()

	// Рисуем статусную строку
	a.drawStatus()

	a.screen.Show()

}

// Отрисовка списка файлов
func (a *App) drawFileList() {
	theme := a.getTheme()

	// Цвет рамки слева — используем left panel fg или общий foreground
	borderColor := parseColor(theme.UI.LeftPanel.FG)
	if borderColor == tcell.ColorDefault {
		borderColor = parseColor(theme.UI.Foreground)
	}
	for y := 0; y < a.height-3; y++ {
		a.screen.SetContent(a.leftWidth, y, '│', nil, tcell.StyleDefault.Foreground(borderColor))
	}

	// Заголовок
	title := "Files"
	col := 0
	titleColor := parseColor(theme.UI.Accent)
	if titleColor == tcell.ColorDefault {
		titleColor = parseColor(theme.UI.Foreground)
	}
	for _, r := range title {
		w := runewidth.RuneWidth(r)
		if col >= a.leftWidth-2 {
			break
		}
		a.screen.SetContent(col+1, 0, r, nil, tcell.StyleDefault.Foreground(titleColor).Bold(true))
		col += w
	}

	// Список файлов
	startY := 2
	visibleHeight := a.height - 5

	for i, file := range a.files {
		if i >= visibleHeight {
			break
		}

		y := startY + i
		if y >= a.height-3 {
			break
		}

		// Выделяем текущий элемент
		style := tcell.StyleDefault
		if i == a.cursor && a.activePanel == "left" {
			// применяем selected из темы
			sfg := theme.UI.LeftPanel.SelectedFG
			sbg := theme.UI.LeftPanel.SelectedBG
			if sfg == "" {
				sfg = "#000000"
			}
			if sbg == "" {
				sbg = theme.UI.Accent
			}
			style = style.Foreground(parseColor(sfg)).Background(parseColor(sbg))
			if theme.UI.LeftPanel.SelectedBold {
				style = style.Bold(true)
			}
		}

		// Имя файла
		name := file.name
		if file.isDir {
			if i == a.cursor && a.activePanel == "left" {
				// выделенная директория
				dirFG := theme.UI.LeftPanel.SelectedDirFG
				if dirFG == "" {
					dirFG = theme.UI.LeftPanel.SelectedFG
				}
				style = style.Foreground(parseColor(dirFG))
			} else {
				// обычная директория
				dirFG := theme.UI.LeftPanel.DirFG
				if dirFG == "" {
					dirFG = theme.UI.Accent
				}
				style = style.Foreground(parseColor(dirFG))
			}
		}

		// Обрезаем имя если слишком длинное (учитываем видимую ширину)
		maxCols := a.leftWidth - 2
		displayName := runewidth.Truncate(name, maxCols, "...")

		col := 0
		for _, r := range displayName {
			w := runewidth.RuneWidth(r)
			if col >= maxCols {
				break
			}
			a.screen.SetContent(col+1, y, r, nil, style)
			col += w
		}
	}

}

// Отрисовка редактора
func (a *App) drawEditor() {
	theme := a.getTheme()

	// Заголовок правой панели
	title := "  Editor"
	if a.mode == "preview" {
		title = "  Preview"
	}

	// Добавляем звездочку, если файл был изменен
	if a.fileModified && a.mode == "edit" {
		title += " *"
	}

	maxTitleCols := a.width - a.leftWidth - 2
	if maxTitleCols < 0 {
		maxTitleCols = 0
	}
	col := 0
	// цвет заголовка правой панели — берем RightPanel FG или общий Foreground
	titleColor := parseColor(theme.UI.RightPanel.FG)
	if titleColor == tcell.ColorDefault {
		titleColor = parseColor(theme.UI.Foreground)
	}
	for _, r := range title {
		w := runewidth.RuneWidth(r)
		if col >= maxTitleCols {
			break
		}
		a.screen.SetContent(a.leftWidth+1+col, 0, r, nil, tcell.StyleDefault.Foreground(titleColor).Bold(true))
		col += w
	}

	// Показываем редактор или предпросмотр в зависимости от режима
	if a.mode == "edit" {
		a.drawTextEditor()
	} else {
		a.drawPreview()
	}

}

// Отрисовка текстового редактора
func (a *App) drawTextEditor() {
	// В первую очередь, убедимся, что курсор виден.
	a.ensureCursorVisible()

	lines := a.getLines()
	// Учитываем отступ здесь
	startX := a.leftWidth + 1 + textEditorPadding
	startY := 2
	editorWidth := a.width - a.leftWidth - 2 - textEditorPadding
	editorHeight := a.height - 5
	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

	theme := a.getTheme()

	for i := 0; i < editorHeight; i++ {
		lineIdx := a.scrollY + i
		y := startY + i
		if lineIdx >= len(lines) { // пустые строки после конца файла
			// Если курсор находится на пустой строке после текста
			if a.activePanel == "right" && lineIdx == a.editY {
				// Корректируем положение курсора с учетом отступа
				cursorCol := 0 - runesDisplayWidth([]rune(""), a.scrollX) // фактически 0
				cursorX := startX + cursorCol
				cursorY := y
				// Если курсор на пустой строке, но не в первой позиции, нарисуем курсор-пробел
				if cursorX >= startX && cursorX < startX+editorWidth && cursorY == y {
					a.screen.SetContent(cursorX, cursorY, ' ', nil, tcell.StyleDefault.Background(parseColor(theme.UI.Cursor)).Foreground(parseColor(theme.UI.RightPanel.FG)))
				}
			}
			continue // Продолжаем рисовать "пустые строки" или фон, но не содержимое.
		}
		line := lines[lineIdx]
		col := 0

		// Обычная отрисовка без подсветки синтаксиса (подходящая для Markdown plain-editor)
		runes := []rune(line)
		// Итерируем по runes, начиная с rune-индекса scrollX
		for k := a.scrollX; k < len(runes); k++ {
			if col >= editorWidth {
				break
			}
			r := runes[k]
			w := runewidth.RuneWidth(r)
			if col+w > editorWidth {
				break
			}
			style := tcell.StyleDefault

			// Если это активный курсор, инвертируем цвет текущего символа
			if a.activePanel == "right" && lineIdx == a.editY && k == a.editX {
				style = style.Background(parseColor(theme.UI.Cursor)).Foreground(parseColor(theme.UI.RightPanel.FG))
			}
			// Здесь startX уже содержит textEditorPadding
			a.screen.SetContent(startX+col, y, r, nil, style)
			col += w
		}

		// Если курсор находится в конце строки (после последнего символа)
		runes = []rune(line)
		if a.activePanel == "right" && lineIdx == a.editY && a.editX == len(runes) {
			// Корректируем положение курсора с учетом отступа
			// вычисляем дисплей-колонку курсора и курсора прокрутки
			cursorDisp := runesDisplayWidth(runes, a.editX)
			scrollDisp := runesDisplayWidth(runes, a.scrollX)
			cursorX := startX + (cursorDisp - scrollDisp)
			if cursorX >= startX && cursorX < startX+editorWidth {
				a.screen.SetContent(cursorX, y, ' ', nil, tcell.StyleDefault.Background(parseColor(theme.UI.Cursor)).Foreground(parseColor(theme.UI.RightPanel.FG))) // рисуем инвертированный пробел
			}
		}
	}

	// --- управление реальным курсором терминала ---
	// Показываем терминальный курсор, если правая панель активна и курсор внутри видимой области редактора.
	if a.activePanel == "right" {
		if a.editY >= a.scrollY && a.editY < a.scrollY+editorHeight {
			// Получаем строку (если её нет, считаем пустой)
			var line string
			if a.editY < len(lines) {
				line = lines[a.editY]
			} else {
				line = ""
			}
			cursorDisp := runesDisplayWidth([]rune(line), a.editX)
			scrollDisp := runesDisplayWidth([]rune(line), a.scrollX)
			cursorX := startX + (cursorDisp - scrollDisp)
			cursorY := startY + (a.editY - a.scrollY)
			if cursorX >= startX && cursorX < startX+editorWidth && cursorY >= startY && cursorY < startY+editorHeight {
				a.screen.ShowCursor(cursorX, cursorY)
			} else {
				a.screen.HideCursor()
			}
		} else {
			a.screen.HideCursor()
		}
	} else {
		a.screen.HideCursor()
	}

}

// Отрисовка предпросмотра
func (a *App) drawPreview() {
	lines := strings.Split(a.fileContent, "\n")
	startX := a.leftWidth + 1 + textEditorPadding
	startY := 2
	editorWidth := a.width - a.leftWidth - 2 - textEditorPadding
	editorHeight := a.height - 5

	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

	theme := a.getTheme()

	inCodeBlock := false
	// регулярка для списков: -, +, * или N.
	listRe := regexp.MustCompile(`^\s*([-+*]|\d+\.)\s+`)

	for i, line := range lines {
		if i < a.scrollY {
			continue
		}
		y := startY + i - a.scrollY
		if y >= startY+editorHeight {
			break
		}

		trim := strings.TrimRight(line, "\r\n")

		// fence handling
		if strings.HasPrefix(trim, "```") {
			inCodeBlock = !inCodeBlock
			// optionally show language after ```
			continue
		}

		// default base style: используем общий foreground
		baseStyle := tcell.StyleDefault.Foreground(parseColor(theme.UI.Foreground))

		// decide line-level style and possibly trim prefixes
		if inCodeBlock {
			baseStyle = styleFromSpec(theme.Markdown.CodeBlock, theme.UI)
		} else if strings.HasPrefix(trim, "# ") {
			trim = strings.TrimPrefix(trim, "# ")
			baseStyle = styleFromSpec(theme.Markdown.H1, theme.UI)
		} else if strings.HasPrefix(trim, "## ") {
			trim = strings.TrimPrefix(trim, "## ")
			baseStyle = styleFromSpec(theme.Markdown.H2, theme.UI)
		} else if strings.HasPrefix(trim, "### ") {
			trim = strings.TrimPrefix(trim, "### ")
			baseStyle = styleFromSpec(theme.Markdown.H3, theme.UI)
		} else if strings.HasPrefix(strings.TrimLeft(trim, " "), "> ") {
			// blockquote, keep indentation
			// remove one leading '>' if present after spaces
			idx := strings.Index(trim, "> ")
			if idx >= 0 {
				trim = strings.TrimSpace(trim[idx+2:])
			}
			baseStyle = styleFromSpec(theme.Markdown.Blockquote, theme.UI)
		} else if listRe.MatchString(trim) {
			// don't strip marker completely; will color marker when rendering
			baseStyle = styleFromSpec(theme.Markdown.ListMarker, theme.UI)
		}

		// render line rune-by-rune with inline parsing for `code`, *em* and links
		runes := []rune(trim)
		col := 0
		inInlineCode := false
		inEmphasis := false

		// Итерируем по runes, начиная с rune-индекса scrollX (горизонтальная прокрутка)
		for idx := a.scrollX; idx < len(runes) && col < editorWidth; idx++ {
			r := runes[idx]

			// handle inline code delimiter `
			if r == '`' && !inCodeBlock {
				inInlineCode = !inInlineCode
				continue // don't render the backtick itself
			}

			// handle emphasis markers simple: *text* or _text_
			if (r == '*' || r == '_') && !inInlineCode {
				prevIsSpace := idx == 0 || runes[idx-1] == ' ' || runes[idx-1] == '\t'
				nextIsSpace := idx+1 >= len(runes) || runes[idx+1] == ' ' || runes[idx+1] == '\t'
				if !prevIsSpace && !nextIsSpace {
					inEmphasis = !inEmphasis
					continue // don't render marker
				}
			}

			// handle links [text](url)
			if r == '[' && !inInlineCode {
				// find closing ] and opening ( and closing )
				closeIdx := -1
				for j := idx + 1; j < len(runes); j++ {
					if runes[j] == ']' {
						closeIdx = j
						break
					}
				}
				if closeIdx != -1 && closeIdx+1 < len(runes) && runes[closeIdx+1] == '(' {
					// find closing )
					parenClose := -1
					for j := closeIdx + 2; j < len(runes); j++ {
						if runes[j] == ')' {
							parenClose = j
							break
						}
					}
					if parenClose != -1 {
						// render the text between idx+1 .. closeIdx-1 as link text
						linkText := runes[idx+1 : closeIdx]
						// Применяем горизонтальную прокрутку к тексту ссылки
						linkCol := 0
						linkStyle := styleFromSpec(theme.Markdown.Link, theme.UI)
						for k := 0; k < len(linkText) && linkCol < editorWidth-col; k++ {
							lr := linkText[k]
							w := runewidth.RuneWidth(lr)
							if linkCol+w > editorWidth-col {
								break
							}
							a.screen.SetContent(startX+col+linkCol, y, lr, nil, linkStyle)
							linkCol += w
						}
						col += linkCol
						// advance idx to parenClose (skip url)
						idx = parenClose
						continue
					}
				}
			}

			// choose style for this rune
			curStyle := baseStyle
			if inInlineCode {
				curStyle = styleFromSpec(theme.Markdown.InlineCode, theme.UI)
			} else if inEmphasis {
				curStyle = curStyle.Bold(true)
			}

			// special: color list marker differently if at line start
			// Учитываем смещение при горизонтальной прокрутке
			if (r == '-' || r == '+' || r == '*') && idx == 0 && listRe.MatchString(string(runes)) {
				curStyle = styleFromSpec(theme.Markdown.ListMarker, theme.UI)
			}

			w := runewidth.RuneWidth(r)
			if col+w > editorWidth {
				break
			}
			a.screen.SetContent(startX+col, y, r, nil, curStyle)
			col += w
		}
	}

}

// Отрисовка статусной строки
func (a *App) drawStatus() {
	y := a.height - 1
	theme := a.getTheme()

	// Определяем цвет для активной панели
	panelColor := parseColor(theme.UI.Foreground)
	if a.activePanel == "left" {
		panelColor = parseColor(theme.UI.LeftPanel.FG)
	} else {
		panelColor = parseColor(theme.UI.RightPanel.FG)
	}

	// Формируем статусную строку с фиксированной шириной для панели и режима
	panelText := fmt.Sprintf("%-5s", a.activePanel) // панель всегда 5 символов (left/right)
	modeText := fmt.Sprintf("%-8s", a.mode)         // режим всегда 7 символов (edit/preview)
	status := fmt.Sprintf("Panel: %s | Mode: %s | File: %s", panelText, modeText, filepath.Base(a.currentFile))

	col := 0
	panelStart := runewidth.StringWidth("Panel: ")
	modeStart := 21 // "Panel: " (7) + 5 символов панели + " | Mode: " (8)
	for _, r := range status {
		w := runewidth.RuneWidth(r)
		if col >= a.width {
			break
		}
		style := tcell.StyleDefault.Foreground(parseColor(theme.UI.Statusbar.FG))
		// Проверяем, находится ли символ в области панели
		if col >= panelStart && col < panelStart+5 {
			style = style.Foreground(panelColor).Bold(true)
		}
		// Проверяем, находится ли символ в области режима
		if col >= modeStart && col < modeStart+8 {
			// Определяем цвет для активного режима
			color := parseColor(theme.UI.Statusbar.FG)
			if a.mode == "edit" {
				color = parseColor(theme.UI.RightPanel.FG)
			} else {
				color = parseColor(theme.UI.LeftPanel.FG)
			}
			style = style.Foreground(color).Bold(true)
		}
		a.screen.SetContent(col, y, r, nil, style)
		col += w
	}

}

// Обработка событий клавиатуры
func (a *App) handleKey(ev *tcell.EventKey) {
	doBackspace := func() {
		if a.activePanel != "right" || a.mode != "edit" {
			return
		}
		lines := a.getLines()
		if len(lines) == 0 {
			lines = []string{""}
			a.setLines(lines)
			a.editY = 0
			a.editX = 0
			a.ensureCursorVisible()
			return
		}

		line := lines[a.editY]
		runes := []rune(line)
		if a.editX > 0 {
			if a.editX <= len(runes) {
				lines[a.editY] = string(append(runes[:a.editX-1], runes[a.editX:]...))
				a.setLines(lines)
				a.editX--
			}
		} else if a.editY > 0 {
			prev := lines[a.editY-1]
			lines[a.editY-1] = prev + line
			newLines := append([]string{}, lines[:a.editY]...)
			if a.editY+1 <= len(lines)-1 {
				newLines = append(newLines, lines[a.editY+1:]...)
			}
			a.setLines(newLines)
			a.editY--
			a.editX = len([]rune(prev))
		}
		a.ensureCursorVisible()
	}

	doDelete := func() {
		if a.activePanel != "right" || a.mode != "edit" {
			return
		}
		lines := a.getLines()
		if len(lines) == 0 {
			return
		}
		line := lines[a.editY]
		runes := []rune(line)
		if a.editX < len(runes) {
			lines[a.editY] = string(append(runes[:a.editX], runes[a.editX+1:]...))
			a.setLines(lines)
		} else if a.editY < len(lines)-1 {
			next := lines[a.editY+1]
			lines[a.editY] = line + next
			newLines := append([]string{}, lines[:a.editY+1]...)
			if a.editY+2 <= len(lines)-1 {
				newLines = append(newLines, lines[a.editY+2:]...)
			}
			a.setLines(newLines)
		}
		a.ensureCursorVisible()
	}

	if ev.Key() == tcell.KeyBackspace || ev.Key() == tcell.KeyBackspace2 || ev.Rune() == '\b' {
		doBackspace()
		return
	}
	if ev.Key() == tcell.KeyDelete || ev.Rune() == rune(127) {
		// Обработка Delete в зависимости от активной панели
		if a.activePanel == "left" {
			a.deleteFile()
		} else {
			doDelete()
		}
		return
	}

	// Общие команды
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		a.screen.Fini()
		os.Exit(0)
	case tcell.KeyCtrlS:
		a.saveFile()
	case tcell.KeyTab:
		// Переключаем между режимами редактирования и предпросмотра
		if a.activePanel == "right" {
			a.toggleMode()
		}
	case tcell.KeyCtrlT:
		a.toggleTerminal() // новый вызов терминала
	case tcell.KeyCtrlR:
		// перезагрузка темы вручную
		a.reloadTheme()
	}

	// Переключение панелей Ctrl+стрелки
	if ev.Modifiers()&tcell.ModCtrl != 0 {
		switch ev.Key() {
		case tcell.KeyLeft:
			a.setActivePanel("left")
			return
		case tcell.KeyRight:
			a.setActivePanel("right")
			return
		}
	}

	// Навигация стрелками/Enter
	switch ev.Key() {
	case tcell.KeyUp:
		if a.activePanel == "left" && a.cursor > 0 {
			a.cursor--
		} else if a.activePanel == "right" {
			lines := a.getLines()
			if a.mode == "edit" && a.editY > 0 {
				a.editY--
				if a.editX > len([]rune(lines[a.editY])) {
					a.editX = len([]rune(lines[a.editY]))
				}
				a.ensureCursorVisible()
			} else if a.mode == "preview" && a.scrollY > 0 {
				a.scrollY--
			}
		}
	case tcell.KeyDown:
		if a.activePanel == "left" && a.cursor < len(a.files)-1 {
			a.cursor++
		} else if a.activePanel == "right" {
			lines := a.getLines()
			if a.mode == "edit" && a.editY < len(lines)-1 {
				a.editY++
				if a.editX > len([]rune(lines[a.editY])) {
					a.editX = len([]rune(lines[a.editY]))
				}
				a.ensureCursorVisible()
			} else if a.mode == "preview" && a.scrollY < len(lines)-1 {
				a.scrollY++
			}
		}
	case tcell.KeyLeft:
		if a.activePanel == "left" {
			a.goBack()
		} else if a.activePanel == "right" {
			if a.mode == "edit" {
				if a.editX > 0 {
					a.editX--
				} else if a.editY > 0 {
					a.editY--
					a.editX = len([]rune(a.getLines()[a.editY]))
				}
				a.ensureCursorVisible()
			} else if a.mode == "preview" && a.scrollX > 0 {
				a.scrollX--
			}
		}
	case tcell.KeyRight:
		if a.activePanel == "left" {
			a.openSelected()
		} else if a.activePanel == "right" {
			lines := a.getLines()
			if a.mode == "edit" {
				lineLen := len([]rune(lines[a.editY]))
				if a.editX < lineLen {
					a.editX++
				} else if a.editY < len(lines)-1 {
					a.editY++
					a.editX = 0
				}
				a.ensureCursorVisible()
			} else if a.mode == "preview" {
				a.scrollX++
			}
		}
	case tcell.KeyEnter:
		if a.activePanel == "left" {
			a.openSelected()
		} else if a.activePanel == "right" && a.mode == "edit" {
			lines := a.getLines()
			line := lines[a.editY]
			runes := []rune(line)
			left := string(runes[:a.editX])
			right := string(runes[a.editX:])
			lines[a.editY] = left
			newLines := append([]string{}, lines[:a.editY+1]...)
			newLines = append(newLines, right)
			if a.editY+1 < len(lines) {
				newLines = append(newLines, lines[a.editY+1:]...)
			}
			a.setLines(newLines)
			a.editY++
			a.editX = 0
			a.ensureCursorVisible()
		}
	}

	// Ввод символов
	if ev.Rune() != 0 {
		r := ev.Rune()
		switch r {
		case '.':
			a.toggleHidden()
			return
		case '?':
			a.showHelp()
			return
		}

		if a.activePanel == "right" && a.mode == "edit" {
			lines := a.getLines()
			if len(lines) == 0 {
				lines = []string{""}
			}
			line := lines[a.editY]
			runes := []rune(line)
			if a.editX < 0 {
				a.editX = 0
			}
			if a.editX > len(runes) {
				a.editX = len(runes)
			}

			// Вставляем символ
			lines[a.editY] = string(append(append(runes[:a.editX], r), runes[a.editX:]...))
			a.editX++

			// Для Markdown не выполняем специальные авто-отступы как для Go
			a.setLines(lines)
			a.ensureCursorVisible()
		}
	}

}

// Основной цикл приложения
func (a *App) Run() {
	for {
		a.draw()

		ev := a.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			a.handleKey(ev)
		case *tcell.EventResize:
			a.screen.Sync()
		}
	}

}

func main() {
	app, err := NewApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка инициализации: %v\n", err)
		os.Exit(1)
	}
	defer app.screen.Fini()

	app.Run()

}
