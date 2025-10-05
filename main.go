package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Структура для хранения информации о файле
type fileItem struct {
	name  string
	path  string
	isDir bool
}

// Основная структура приложения
type App struct {
	screen     tcell.Screen
	currentDir string
	files      []fileItem
	cursor     int
	showHidden bool

	currentFile string
	fileContent string
	mode        string // "edit" или "preview"
	activePanel string // "left" или "right"

	// Размеры экрана
	width, height int

	// Позиции курсора в редакторе (в rune-единицах)
	editX, editY int

	// Смещение для прокрутки (в rune-единицах)
	scrollX, scrollY int

	// Размеры панелей
	leftWidth int
}

// Инициализация приложения
func NewApp() (*App, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}

	if err := screen.Init(); err != nil {
		return nil, err
	}

	app := &App{
		screen:      screen,
		currentDir:  "",
		files:       []fileItem{},
		cursor:      0,
		showHidden:  false,
		currentFile: "",
		fileContent: "",
		mode:        "edit",
		activePanel: "left",
		editX:       0,
		editY:       0,
		scrollX:     0,
		scrollY:     0,
		leftWidth:   30,
	}

	// Получаем текущую директорию
	if cwd, err := os.Getwd(); err == nil {
		app.currentDir = cwd
	}

	app.loadFiles()
	return app, nil
}

// Загрузка файлов из текущей директории
func (a *App) loadFiles() {
	a.files = []fileItem{}

	// Добавляем родительскую директорию
	if parent := filepath.Dir(a.currentDir); parent != a.currentDir {
		a.files = append(a.files, fileItem{
			name:  "..",
			path:  parent,
			isDir: true,
		})
	}

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

// Открытие файла для редактирования
func (a *App) openFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		a.fileContent = fmt.Sprintf("Ошибка чтения файла: %v", err)
		return
	}

	a.currentFile = path
	a.fileContent = string(content)
	a.editX = 0
	a.editY = 0
	a.scrollX = 0
	a.scrollY = 0
	a.clampCursor()
}

// Сохранение текущего файла
func (a *App) saveFile() {
	if a.currentFile == "" {
		return
	}

	err := os.WriteFile(a.currentFile, []byte(a.fileContent), 0644)
	if err != nil {
		// Можно добавить уведомление об ошибке
		return
	}
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
Esc - вернуться в левую панель
Ctrl+H - переключить на левую панель
Ctrl+L - переключить на правую панель
← (в правой панели) - вернуться в левую панель
↑ (в правой панели) - вернуться в левую панель

РЕДАКТИРОВАНИЕ:
Tab - переключить режим редактирования/предпросмотра
Ctrl+S - сохранить файл

ПРОЧЕЕ:
. - показать/скрыть скрытые файлы
? - показать справку
Ctrl+Q - выйти

Нажмите любую клавишу для закрытия справки...`

	// Временно заменяем содержимое на справку
	oldContent := a.fileContent
	oldMode := a.mode
	a.fileContent = helpText
	a.mode = "preview"
	a.draw()

	// Ждем нажатия клавиши
	ev := a.screen.PollEvent()
	if _, ok := ev.(*tcell.EventKey); ok {
		// Восстанавливаем содержимое
		a.fileContent = oldContent
		a.mode = oldMode
	}
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

// Установить строки обратно в fileContent
func (a *App) setLines(lines []string) {
	a.fileContent = strings.Join(lines, "\n")
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

// Обеспечить видимость курсора (корректирует scrollX/Y)
func (a *App) ensureCursorVisible() {
	a.width, a.height = a.screen.Size()
	editorWidth := a.width - a.leftWidth - 2
	editorHeight := a.height - 3
	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

	if a.editY < a.scrollY {
		a.scrollY = a.editY
	} else if a.editY >= a.scrollY+editorHeight {
		a.scrollY = a.editY - editorHeight + 1
	}

	if a.editX < a.scrollX {
		a.scrollX = a.editX
	} else if a.editX >= a.scrollX+editorWidth {
		a.scrollX = a.editX - editorWidth + 1
	}

	if a.scrollY < 0 {
		a.scrollY = 0
	}
	if a.scrollX < 0 {
		a.scrollX = 0
	}
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
	// Рамка левой панели
	for y := 0; y < a.height-1; y++ {
		a.screen.SetContent(a.leftWidth, y, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorGrey))
	}

	// Заголовок
	title := "Files"
	for i, r := range title {
		if i < a.leftWidth-2 {
			a.screen.SetContent(i+1, 0, r, nil, tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true))
		}
	}

	// Список файлов
	startY := 2
	visibleHeight := a.height - 3

	for i, file := range a.files {
		if i >= visibleHeight {
			break
		}

		y := startY + i
		if y >= a.height-1 {
			break
		}

		// Выделяем текущий элемент
		style := tcell.StyleDefault
		if i == a.cursor && a.activePanel == "left" {
			style = style.Background(tcell.ColorRed).Foreground(tcell.ColorBlack)
		}

		// Имя файла
		name := file.name
		if file.isDir {
			name += "/"
			style = style.Foreground(tcell.ColorBlue)
		}

		// Обрезаем имя если слишком длинное
		if len(name) > a.leftWidth-2 {
			name = name[:a.leftWidth-5] + "..."
		}

		for j, r := range name {
			if j < a.leftWidth-2 {
				a.screen.SetContent(j+1, y, r, nil, style)
			}
		}
	}
}

// Отрисовка редактора
func (a *App) drawEditor() {
	// Заголовок правой панели
	title := "Editor"
	if a.mode == "preview" {
		title = "Preview"
	}

	for i, r := range title {
		if i < a.width-a.leftWidth-2 {
			a.screen.SetContent(a.leftWidth+1+i, 0, r, nil, tcell.StyleDefault.Foreground(tcell.ColorGreen).Bold(true))
		}
	}

	if a.mode == "edit" {
		a.drawTextEditor()
	} else {
		a.drawPreview()
	}
}

// Отрисовка текстового редактора
func (a *App) drawTextEditor() {
	a.ensureCursorVisible()
	lines := a.getLines()
	startX := a.leftWidth + 1
	startY := 2
	editorWidth := a.width - a.leftWidth - 2
	editorHeight := a.height - 3
	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

	for i := 0; i < editorHeight; i++ {
		lineIdx := a.scrollY + i
		y := startY + i
		if lineIdx >= len(lines) { // пустые строки после конца файла
			// Если курсор находится на пустой строке после текста
			if a.activePanel == "right" && lineIdx == a.editY {
				cursorX := startX + a.editX - a.scrollX
				cursorY := y
				// Если курсор на пустой строке, но не в первой позиции, нарисуем курсор-пробел
				if cursorX >= startX && cursorX < startX+editorWidth && cursorY == y {
					a.screen.SetContent(cursorX, cursorY, ' ', nil, tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack))
				}
			}
			continue // Продолжаем рисовать "пустые строки" или фон, но не содержимое.
		}
		line := lines[lineIdx]
		runes := []rune(line)
		col := 0
		for k := a.scrollX; k < len(runes); k++ { // Итерируем по всем rune
			if col == editorWidth { // Если достигли края экрана, прекращаем отрисовку строки
				break
			}
			r := runes[k]
			style := tcell.StyleDefault

			// Если это активный курсор, инвертируем цвет текущего символа
			if a.activePanel == "right" && lineIdx == a.editY && k == a.editX {
				style = style.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
			}
			a.screen.SetContent(startX+col, y, r, nil, style)
			col++
		}

		// Если курсор находится в конце строки (после последнего символа)
		if a.activePanel == "right" && lineIdx == a.editY && a.editX == len(runes) {
			cursorX := startX + a.editX - a.scrollX
			if cursorX >= startX && cursorX < startX+editorWidth {
				a.screen.SetContent(cursorX, y, ' ', nil, tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)) // рисуем инвертированный пробел
			}
		}
	}
}

// Отрисовка предпросмотра
func (a *App) drawPreview() {
	lines := strings.Split(a.fileContent, "\n")
	startX := a.leftWidth + 1
	startY := 2
	editorWidth := a.width - a.leftWidth - 2
	editorHeight := a.height - 3

	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

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

		// default base style
		baseStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)

		// decide line-level style and possibly trim prefixes
		if inCodeBlock {
			baseStyle = tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlack)
		} else if strings.HasPrefix(trim, "# ") {
			trim = strings.TrimPrefix(trim, "# ")
			baseStyle = tcell.StyleDefault.Foreground(tcell.ColorGreen).Bold(true)
		} else if strings.HasPrefix(trim, "## ") {
			trim = strings.TrimPrefix(trim, "## ")
			baseStyle = tcell.StyleDefault.Foreground(tcell.ColorGreen).Bold(true)
		} else if strings.HasPrefix(trim, "### ") {
			trim = strings.TrimPrefix(trim, "### ")
			baseStyle = tcell.StyleDefault.Foreground(tcell.ColorOlive).Bold(true)
		} else if strings.HasPrefix(strings.TrimLeft(trim, " "), "> ") {
			// blockquote, keep indentation
			// remove one leading '>' if present after spaces
			idx := strings.Index(trim, "> ")
			if idx >= 0 {
				trim = strings.TrimSpace(trim[idx+2:])
			}
			baseStyle = tcell.StyleDefault.Foreground(tcell.ColorBlue).Italic(true)
		} else if listRe.MatchString(trim) {
			// don't strip marker completely; will color marker when rendering
			baseStyle = tcell.StyleDefault.Foreground(tcell.ColorGray)
		}

		// render line rune-by-rune with inline parsing for `code`, *em* and links
		runes := []rune(trim)
		col := 0
		inInlineCode := false
		inEmphasis := false

		for idx := 0; idx < len(runes) && col < editorWidth; idx++ {
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
						for _, lr := range linkText {
							if col >= editorWidth {
								break
							}
							linkStyle := baseStyle.Foreground(tcell.ColorLightBlue).Underline(true)
							a.screen.SetContent(startX+col, y, lr, nil, linkStyle)
							col++
						}
						// advance idx to parenClose (skip url)
						idx = parenClose
						continue
					}
				}
			}

			// choose style for this rune
			curStyle := baseStyle
			if inInlineCode {
				curStyle = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
			} else if inEmphasis {
				curStyle = curStyle.Bold(true)
			}

			// special: color list marker differently if at line start
			if (r == '-' || r == '+' || r == '*') && col == 0 && listRe.MatchString(string(runes)) {
				curStyle = tcell.StyleDefault.Foreground(tcell.ColorLightGrey).Bold(true)
			}

			a.screen.SetContent(startX+col, y, r, nil, curStyle)
			col++
		}
	}

}

// Отрисовка статусной строки
func (a *App) drawStatus() {
	y := a.height - 1

	// Определяем цвет для активной панели
	panelColor := tcell.ColorGray
	if a.activePanel == "left" {
		panelColor = tcell.ColorBlue
	} else {
		panelColor = tcell.ColorGreen
	}

	status := fmt.Sprintf("Panel: %s   | Mode: %s    | File: %s", a.activePanel, a.mode, filepath.Base(a.currentFile))

	for i, r := range status {
		if i < a.width {
			style := tcell.StyleDefault.Foreground(tcell.ColorGray)
			if i >= 7 && i < 7+len(a.activePanel) {
				style = style.Foreground(panelColor).Bold(true)
			}
			a.screen.SetContent(i, y, r, nil, style)
		}
	}
}

// Обработка событий клавиатуры
func (a *App) handleKey(ev *tcell.EventKey) {
	// Вспомогательные обработчики для Backspace и Delete
	doBackspace := func() {
		if a.activePanel != "right" || a.mode != "edit" {
			return
		}
		lines := a.getLines()
		// защита
		if len(lines) == 0 {
			a.setLines([]string{""})
			a.editX = 0
			a.editY = 0
			a.ensureCursorVisible()
			return
		}
		line := lines[a.editY]
		runes := []rune(line)
		if a.editX > 0 {
			// удалить rune слева от курсора
			if a.editX <= len(runes) {
				newRunes := append(runes[:a.editX-1], runes[a.editX:]...)
				lines[a.editY] = string(newRunes)
				a.setLines(lines)
				a.editX--
			}
		} else {
			// если в начале строки — объединить с предыдущей
			if a.editY > 0 {
				prev := lines[a.editY-1]
				lines[a.editY-1] = prev + line
				// удалить текущую строку
				newLines := append([]string{}, lines[:a.editY]...)
				if a.editY+1 <= len(lines)-1 {
					newLines = append(newLines, lines[a.editY+1:]...)
				}
				a.setLines(newLines)
				a.editY--
				a.editX = len([]rune(prev))
			}
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
			// удалить текущий rune
			newRunes := append(runes[:a.editX], runes[a.editX+1:]...)
			lines[a.editY] = string(newRunes)
			a.setLines(lines)
		} else {
			// если в конце строки — объединить с следующей
			if a.editY < len(lines)-1 {
				next := lines[a.editY+1]
				lines[a.editY] = line + next
				// удалить следующую строку
				newLines := append([]string{}, lines[:a.editY+1]...)
				if a.editY+2 <= len(lines)-1 {
					newLines = append(newLines, lines[a.editY+2:]...)
				}
				a.setLines(newLines)
			}
		}
		a.ensureCursorVisible()
	}

	// Нормализация Backspace/DEL/Ctrl-H: направляем разные варианты в соответствующие обработчики.
	// Сценарий: Ctrl-H в большинстве терминалов может приходить как Backspace.
	// Если это Ctrl-H и пользователь в режиме редактирования правой панели — считаем Backspace.
	if ev.Key() == tcell.KeyBackspace || ev.Key() == tcell.KeyBackspace2 || ev.Rune() == '\b' {
		doBackspace()
		return
	}
	// Ctrl-H может приходить как tcell.KeyCtrlH; если в edit на правой панели — обрабатываем как backspace,
	// иначе используем его как переключение панели (чтобы не ломать пользовательские хоткеи).
	if ev.Key() == tcell.KeyCtrlH {
		if a.activePanel == "right" && a.mode == "edit" {
			doBackspace()
			return
		}
		// иначе — переключение на левую панель
		a.setActivePanel("left")
		return
	}
	// DEL: явный KeyDelete или ASCII DEL (127)
	if ev.Key() == tcell.KeyDelete || ev.Rune() == rune(127) {
		doDelete()
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
		a.toggleMode()

	case tcell.KeyEscape:
		a.setActivePanel("left")
	}

	// Навигация и редактирование стрелками/Enter и т.д.
	switch ev.Key() {
	case tcell.KeyUp:
		if a.activePanel == "left" {
			if a.cursor > 0 {
				a.cursor--
			}
		} else if a.activePanel == "right" {
			if a.mode == "edit" {
				if a.editY > 0 {
					a.editY--
					// корректируем X по длине новой строки
					lines := a.getLines()
					lineRunes := []rune(lines[a.editY])
					if a.editX > len(lineRunes) {
						a.editX = len(lineRunes)
					}
					a.ensureCursorVisible()
				} else {
					// Если курсор в самом верху, переходим в левую панель
					a.setActivePanel("left")
				}
			} else {
				// preview -> вернуть в левую панель
				a.setActivePanel("left")
			}
		}
	case tcell.KeyDown:
		if a.activePanel == "left" {
			if a.cursor < len(a.files)-1 {
				a.cursor++
			}
		} else if a.activePanel == "right" && a.mode == "edit" {
			lines := a.getLines()
			if a.editY < len(lines)-1 {
				a.editY++
				// корректируем X по длине новой строки
				lineRunes := []rune(lines[a.editY])
				if a.editX > len(lineRunes) {
					a.editX = len(lineRunes)
				}
				a.ensureCursorVisible()
			}
		}
	case tcell.KeyLeft:
		if a.activePanel == "left" {
			a.goBack()
		} else if a.activePanel == "right" {
			if a.mode == "edit" {
				if a.editX > 0 {
					a.editX--
					a.ensureCursorVisible()
				} else {
					// Переместиться к концу предыдущей строки, если есть
					if a.editY > 0 {
						a.editY--
						lines := a.getLines()
						a.editX = len([]rune(lines[a.editY]))
						a.ensureCursorVisible()
					} else {
						// Если курсор в начале файла, переходим в левую панель
						a.setActivePanel("left")
					}
				}
			} else {
				a.setActivePanel("left")
			}
		}
	case tcell.KeyRight:
		if a.activePanel == "left" {
			a.openSelected()
		} else if a.activePanel == "right" && a.mode == "edit" {
			lines := a.getLines()
			lineRunes := []rune(lines[a.editY])
			if a.editX < len(lineRunes) {
				a.editX++
				a.ensureCursorVisible()
			} else {
				// перейти в начало следующей строки, если есть
				if a.editY < len(lines)-1 {
					a.editY++
					a.editX = 0
					a.ensureCursorVisible()
				}
			}
		}
	case tcell.KeyEnter:
		if a.activePanel == "left" {
			a.openSelected()
		} else if a.activePanel == "right" && a.mode == "edit" {
			// Вставить новую строку
			lines := a.getLines()
			line := lines[a.editY]
			runes := []rune(line)
			left := string(runes[:a.editX])
			right := string(runes[a.editX:])
			// обновляем текущую строку и вставляем новую
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

	// Обработка символов (ввод текста)
	if ev.Rune() != 0 {
		r := ev.Rune()
		switch r {
		case '.':
			// если пользователь нажал просто точку в любой панели
			a.toggleHidden()
			return
		case '?':
			a.showHelp()
			return
		}

		// Ввод символов в режиме редактирования правой панели
		if a.activePanel == "right" && a.mode == "edit" {
			lines := a.getLines()
			// защита: если нет строк, добавим
			if len(lines) == 0 {
				lines = []string{""}
			}
			// вставляем rune
			line := lines[a.editY]
			runes := []rune(line)
			// вставка r в позицию editX
			if a.editX < 0 {
				a.editX = 0
			}
			if a.editX > len(runes) {
				a.editX = len(runes)
			}
			newRunes := append([]rune{}, runes[:a.editX]...)
			newRunes = append(newRunes, r)
			newRunes = append(newRunes, runes[a.editX:]...)
			lines[a.editY] = string(newRunes)
			a.setLines(lines)
			a.editX++
			a.ensureCursorVisible()
			return
		}

		// Горячие сочетания с буквами (Ctrl+h, Ctrl+l)
		// Если ev.Rune() == 'h' и модификатор Ctrl — переключаем панели.
		// Аналогично для 'l'.
		switch r {
		case 'h':
			if ev.Modifiers()&tcell.ModCtrl != 0 {
				a.setActivePanel("left")
			}
		case 'l':
			if ev.Modifiers()&tcell.ModCtrl != 0 {
				a.setActivePanel("right")
			}
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
