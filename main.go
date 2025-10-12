package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
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
	mode         string // "edit" или "preview" (оставлено для совместимости, но preview не показывается)
	activePanel  string // "left" или "right"

	// Размеры экрана
	width, height int

	// Позиции курсора в редакторе (в rune-единицах)
	editX, editY int

	// Смещение для прокрутки (в rune-единицах)
	scrollX, scrollY int

	// Размеры панелей
	leftWidth int
}

// Тип токена для подсветки
type hlToken struct {
	text  string
	style tcell.Style
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
	a.fileModified = false // сбрасываем флаг изменений при открытии файла
	a.editX = 0
	a.editY = 0
	a.scrollX = 0
	a.scrollY = 0
	a.clampCursor()
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

// Форматирование кода с помощью gofmt
func (a *App) formatWithGofmt() {
	if a.currentFile == "" || !a.isGoFile() {
		return
	}

	// Создаем временную команду для форматирования
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(a.fileContent)
	output, err := cmd.Output()

	if err != nil {
		// В случае ошибки форматирования, можно показать уведомление
		// Пока просто возвращаемся без изменений
		return
	}

	// Обновляем содержимое файла отформатированным текстом
	a.fileContent = string(output)
	a.fileModified = true
}

// Форматирование кода с помощью goimports
func (a *App) formatWithGoimports() {
	if a.currentFile == "" || !a.isGoFile() {
		return
	}

	// Создаем временную команду для форматирования
	cmd := exec.Command("goimports")
	cmd.Stdin = strings.NewReader(a.fileContent)
	output, err := cmd.Output()

	if err != nil {
		// В случае ошибки форматирования, можно показать уведомление
		// Пока просто возвращаемся без изменений
		return
	}

	// Обновляем содержимое файла отформатированным текстом
	a.fileContent = string(output)
	a.fileModified = true
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
Tab - переключить режим редактирования/предпросмотра (отключено)
Ctrl+S - сохранить файл
Ctrl+F - форматировать код с помощью gofmt
Ctrl+G - форматировать код с помощью goimports
Delete - удалить файл (в левой панели)

ПРОЧЕЕ:
. - показать/скрыть скрытые файлы
? - показать справку
Ctrl+Q - выйти

ИНДИКАТОРЫ:
* - в заголовке редактора означает, что файл был изменен, но еще не сохранен

ПОДСВЕТКА СИНТАКСА:
Автоматическая подсветка синтаксиса для файлов Go (.go)

Нажмите любую клавишу для закрытия справки...`

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

// Проверить, является ли файл Go файлом
func (a *App) isGoFile() bool {
	return strings.HasSuffix(a.currentFile, ".go")
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
		// нужно подобрать новое scrollX (rune-индекс) так, чтобы курсор поместился
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

// Отрисовка интерфейса
func (a *App) draw() {
	a.screen.Clear()

	// Получаем размеры экрана
	a.width, a.height = a.screen.Size()

	// Рисуем левую панель (файловый менеджер)
	a.drawFileList()

	// Рисуем правую панель (редактор). Preview больше не показываем.
	a.drawEditor()

	// Рисуем статусную строку
	a.drawStatus()

	a.screen.Show()
}

// Отрисовка списка файлов
func (a *App) drawFileList() {
	// Рамка левой панели
	for y := 0; y < a.height-3; y++ {
		a.screen.SetContent(a.leftWidth, y, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorGrey))
	}

	// Заголовок
	title := "Files"
	col := 0
	for _, r := range title {
		w := runewidth.RuneWidth(r)
		if col >= a.leftWidth-2 {
			break
		}
		a.screen.SetContent(col+1, 0, r, nil, tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true))
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
			style = style.Background(tcell.ColorRed).Foreground(tcell.ColorBlack)
		}

		// Имя файла
		name := file.name
		if file.isDir {
			name += "/"
			style = style.Foreground(tcell.ColorBlue)
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
	// Заголовок правой панели
	title := "  Editor"

	// Добавляем звездочку, если файл был изменен
	if a.fileModified {
		title += " *"
	}

	maxTitleCols := a.width - a.leftWidth - 2
	if maxTitleCols < 0 {
		maxTitleCols = 0
	}
	col := 0
	for _, r := range title {
		w := runewidth.RuneWidth(r)
		if col >= maxTitleCols {
			break
		}
		a.screen.SetContent(a.leftWidth+1+col, 0, r, nil, tcell.StyleDefault.Foreground(tcell.ColorGreen).Bold(true))
		col += w
	}

	// Всегда показываем редактор (preview отключен)
	a.drawTextEditor()
}

// Подсветка синтаксиса для Go — построчная версия, сохраняющая состояние между строками
func (a *App) highlightGoSyntaxLines(lines []string) [][]hlToken {
	// Ключевые слова Go
	keywords := map[string]bool{
		"break": true, "case": true, "chan": true, "const": true, "continue": true,
		"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
		"func": true, "go": true, "goto": true, "if": true, "import": true,
		"interface": true, "map": true, "package": true, "range": true, "return": true,
		"select": true, "struct": true, "switch": true, "type": true, "var": true,
	}
	types := map[string]bool{
		"bool": true, "byte": true, "complex64": true, "complex128": true, "error": true,
		"float32": true, "float64": true, "int": true, "int8": true, "int16": true,
		"int32": true, "int64": true, "rune": true, "string": true, "uint": true,
		"uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	}

	var result [][]hlToken

	inString := false       // "
	inRawString := false    // `
	inMultiComment := false // /* */
	inEscape := false       // escape inside normal string

	for _, line := range lines {
		runes := []rune(line)
		i := 0
		var row []hlToken

		for i < len(runes) {
			// Если в многострочном комментарии — ищем конец
			if inMultiComment {
				start := i
				for i < len(runes) {
					if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '/' {
						i += 2
						inMultiComment = false
						break
					}
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorGreen)})
				continue
			}

			// Если в raw string (backticks)
			if inRawString {
				start := i
				for i < len(runes) {
					if runes[i] == '`' {
						i++
						inRawString = false
						break
					}
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorYellow)})
				continue
			}

			// Если внутри обычной строки "
			if inString {
				start := i
				for i < len(runes) {
					if runes[i] == '"' && !inEscape {
						i++
						inString = false
						break
					}
					if runes[i] == '\\' && !inEscape {
						inEscape = true
					} else {
						inEscape = false
					}
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorYellow)})
				continue
			}

			r := runes[i]

			// начало многострочного комментария
			if i+1 < len(runes) && r == '/' && runes[i+1] == '*' {
				start := i
				i += 2
				inMultiComment = true
				for i < len(runes) {
					if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '/' {
						i += 2
						inMultiComment = false
						break
					}
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorGreen)})
				continue
			}

			// однострочный коммент //
			if i+1 < len(runes) && r == '/' && runes[i+1] == '/' {
				row = append(row, hlToken{string(runes[i:]), tcell.StyleDefault.Foreground(tcell.ColorGreen)})
				break
			}

			// raw string start `
			if r == '`' {
				start := i
				i++
				inRawString = true
				for i < len(runes) {
					if runes[i] == '`' {
						i++
						inRawString = false
						break
					}
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorYellow)})
				continue
			}

			// double-quoted string start "
			if r == '"' {
				start := i
				i++
				inString = true
				inEscape = false
				for i < len(runes) {
					if runes[i] == '"' && !inEscape {
						i++
						inString = false
						break
					}
					if runes[i] == '\\' && !inEscape {
						inEscape = true
					} else {
						inEscape = false
					}
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorYellow)})
				continue
			}

			// идентификаторы
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
				start := i
				for i < len(runes) && ((runes[i] >= 'a' && runes[i] <= 'z') ||
					(runes[i] >= 'A' && runes[i] <= 'Z') ||
					(runes[i] >= '0' && runes[i] <= '9') ||
					runes[i] == '_') {
					i++
				}
				word := string(runes[start:i])
				style := tcell.StyleDefault.Foreground(tcell.ColorWhite)
				if keywords[word] {
					style = tcell.StyleDefault.Foreground(tcell.ColorBlue).Bold(true)
				} else if types[word] {
					style = tcell.StyleDefault.Foreground(tcell.ColorAqua)
				}
				row = append(row, hlToken{word, style})
				continue
			}

			// числа
			if r >= '0' && r <= '9' {
				start := i
				for i < len(runes) && ((runes[i] >= '0' && runes[i] <= '9') ||
					runes[i] == '.' || runes[i] == 'e' || runes[i] == 'E' ||
					runes[i] == '+' || runes[i] == '-') {
					i++
				}
				row = append(row, hlToken{string(runes[start:i]), tcell.StyleDefault.Foreground(tcell.ColorFuchsia)})
				continue
			}

			// прочие одиночные символы
			row = append(row, hlToken{string(r), tcell.StyleDefault.Foreground(tcell.ColorWhite)})
			i++
		}

		result = append(result, row)
	}
	return result
}

// Подсветка синтаксиса для одной строки (устаревшая версия, оставлена для совместимости)
func (a *App) highlightGoSyntax(line string) []struct {
	text  string
	style tcell.Style
} {
	// Ключевые слова Go
	keywords := map[string]bool{
		"break":       true,
		"case":        true,
		"chan":        true,
		"const":       true,
		"continue":    true,
		"default":     true,
		"defer":       true,
		"else":        true,
		"fallthrough": true,
		"for":         true,
		"func":        true,
		"go":          true,
		"goto":        true,
		"if":          true,
		"import":      true,
		"interface":   true,
		"map":         true,
		"package":     true,
		"range":       true,
		"return":      true,
		"select":      true,
		"struct":      true,
		"switch":      true,
		"type":        true,
		"var":         true,
	}

	// Типы Go
	types := map[string]bool{
		"bool":       true,
		"byte":       true,
		"complex64":  true,
		"complex128": true,
		"error":      true,
		"float32":    true,
		"float64":    true,
		"int":        true,
		"int8":       true,
		"int16":      true,
		"int32":      true,
		"int64":      true,
		"rune":       true,
		"string":     true,
		"uint":       true,
		"uint8":      true,
		"uint16":     true,
		"uint32":     true,
		"uint64":     true,
		"uintptr":    true,
	}

	var result []struct {
		text  string
		style tcell.Style
	}

	// Простой парсер для подсветки синтаксиса
	inString := false
	inSingleLineComment := false
	inMultiLineComment := false
	inEscape := false

	runes := []rune(line)
	i := 0

	for i < len(runes) {
		r := runes[i]

		// Обработка многострочных комментариев
		if !inString && !inSingleLineComment && !inMultiLineComment && i+1 < len(runes) && r == '/' && runes[i+1] == '*' {
			// Начало многострочного комментария
			start := i
			i += 2
			for i < len(runes) {
				if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			result = append(result, struct {
				text  string
				style tcell.Style
			}{
				text:  string(runes[start:i]),
				style: tcell.StyleDefault.Foreground(tcell.ColorGreen),
			})
			continue
		}

		// Обработка однострочных комментариев
		if !inString && !inMultiLineComment && i+1 < len(runes) && r == '/' && runes[i+1] == '/' {
			result = append(result, struct {
				text  string
				style tcell.Style
			}{
				text:  string(runes[i:]),
				style: tcell.StyleDefault.Foreground(tcell.ColorGreen),
			})
			break
		}

		// Обработка строк
		if !inSingleLineComment && !inMultiLineComment && r == '"' && !inEscape {
			if !inString {
				// Начало строки
				start := i
				inString = true
				i++
				for i < len(runes) {
					if runes[i] == '"' && !inEscape {
						inString = false
						i++
						break
					}
					if runes[i] == '\\' && !inEscape {
						inEscape = true
					} else {
						inEscape = false
					}
					i++
				}
				result = append(result, struct {
					text  string
					style tcell.Style
				}{
					text:  string(runes[start:i]),
					style: tcell.StyleDefault.Foreground(tcell.ColorYellow),
				})
				continue
			}
		}

		// Обработка escape-последовательностей в строках
		if inString && r == '\\' && !inEscape {
			inEscape = true
		} else {
			inEscape = false
		}

		// Если мы внутри строки или комментария, просто добавляем символ
		if inString || inSingleLineComment || inMultiLineComment {
			i++
			continue
		}

		// Обработка идентификаторов (ключевые слова, типы, имена переменных)
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			start := i
			for i < len(runes) && ((runes[i] >= 'a' && runes[i] <= 'z') ||
				(runes[i] >= 'A' && runes[i] <= 'Z') ||
				(runes[i] >= '0' && runes[i] <= '9') ||
				runes[i] == '_') {
				i++
			}
			word := string(runes[start:i])

			// Определяем стиль для слова
			style := tcell.StyleDefault.Foreground(tcell.ColorWhite)
			if keywords[word] {
				style = tcell.StyleDefault.Foreground(tcell.ColorBlue).Bold(true)
			} else if types[word] {
				style = tcell.StyleDefault.Foreground(tcell.ColorAqua)
			}

			result = append(result, struct {
				text  string
				style tcell.Style
			}{
				text:  word,
				style: style,
			})
			continue
		}

		// Обработка чисел
		if r >= '0' && r <= '9' {
			start := i
			// Простая обработка целых и вещественных чисел
			for i < len(runes) && ((runes[i] >= '0' && runes[i] <= '9') ||
				runes[i] == '.' || runes[i] == 'e' || runes[i] == 'E' ||
				runes[i] == '+' || runes[i] == '-') {
				i++
			}
			result = append(result, struct {
				text  string
				style tcell.Style
			}{
				text:  string(runes[start:i]),
				style: tcell.StyleDefault.Foreground(tcell.ColorFuchsia),
			})
			continue
		}

		// Все остальные символы добавляем как есть
		result = append(result, struct {
			text  string
			style tcell.Style
		}{
			text:  string(r),
			style: tcell.StyleDefault.Foreground(tcell.ColorWhite),
		})
		i++
	}

	return result
}

// Отрисовка текстового редактора
func (a *App) drawTextEditor() {
	// В первую очередь, убедимся, что курсор виден.
	a.ensureCursorVisible()

	lines := a.getLines()
	// Учитываем отступ здесь
	startX := a.leftWidth + 1 + textEditorPadding // <<--- ИЗМЕНЕНИЕ ЗДЕСЬ
	startY := 2
	editorWidth := a.width - a.leftWidth - 2 - textEditorPadding // <<--- ИЗМЕНЕНИЕ ЗДЕСЬ
	editorHeight := a.height - 5
	if editorWidth < 1 {
		editorWidth = 1
	}
	if editorHeight < 1 {
		editorHeight = 1
	}

	// Проверяем, является ли файл Go файлом для подсветки синтаксиса
	isGoFile := a.isGoFile()

	// Предподсветка всего буфера (чтобы сохранить состояния между строками)
	var highlightedLines [][]hlToken
	if isGoFile {
		highlightedLines = a.highlightGoSyntaxLines(lines)
	}

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
					a.screen.SetContent(cursorX, cursorY, ' ', nil, tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack))
				}
			}
			continue // Продолжаем рисовать "пустые строки" или фон, но не содержимое.
		}
		line := lines[lineIdx]
		col := 0

		if isGoFile {
			// Используем подсветку синтаксиса для Go файлов
			highlighted := highlightedLines[lineIdx]
			currentCol := 0

			for _, token := range highlighted {
				runes := []rune(token.text)
				for k := 0; k < len(runes); k++ {
					if currentCol < a.scrollX {
						currentCol++
						continue
					}

					if col >= editorWidth {
						break
					}

					r := runes[k]
					w := runewidth.RuneWidth(r)
					if col+w > editorWidth {
						break
					}

					style := token.style

					// Если это активный курсор, инвертируем цвет текущего символа
					if a.activePanel == "right" && lineIdx == a.editY && currentCol == a.editX {
						style = style.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
					}

					// Здесь startX уже содержит textEditorPadding
					a.screen.SetContent(startX+col, y, r, nil, style)
					col += w
					currentCol++
				}

				if col >= editorWidth {
					break
				}
			}
		} else {
			// Обычная отрисовка без подсветки синтаксиса
			runes := []rune(line)
			// Итерируем по runes, начиная с rune-индекса scrollX
			for k := a.scrollX; k < len(runes); k++ { // Итерируем по всем rune
				if col >= editorWidth { // Если достигли края экрана, прекращаем отрисовку строки (было ==, лучше >=)
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
					style = style.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
				}
				// Здесь startX уже содержит textEditorPadding
				a.screen.SetContent(startX+col, y, r, nil, style)
				col += w
			}
		}

		// Если курсор находится в конце строки (после последнего символа)
		runes := []rune(line)
		if a.activePanel == "right" && lineIdx == a.editY && a.editX == len(runes) {
			// Корректируем положение курсора с учетом отступа
			// вычисляем дисплей-колонку курсора и курсора прокрутки
			cursorDisp := runesDisplayWidth(runes, a.editX)
			scrollDisp := runesDisplayWidth(runes, a.scrollX)
			cursorX := startX + (cursorDisp - scrollDisp)
			if cursorX >= startX && cursorX < startX+editorWidth {
				a.screen.SetContent(cursorX, y, ' ', nil, tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)) // рисуем инвертированный пробел
			}
		}
	}
}

// Отрисовка предпросмотра (осталась, но не используется по умолчанию)
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
						for k := 0; k < len(linkText) && linkCol < editorWidth-col; k++ {
							lr := linkText[k]
							w := runewidth.RuneWidth(lr)
							if linkCol+w > editorWidth-col {
								break
							}
							linkStyle := baseStyle.Foreground(tcell.ColorLightBlue).Underline(true)
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
				curStyle = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
			} else if inEmphasis {
				curStyle = curStyle.Bold(true)
			}

			// special: color list marker differently if at line start
			// Учитываем смещение при горизонтальной прокрутке
			if (r == '-' || r == '+' || r == '*') && idx == 0 && listRe.MatchString(string(runes)) {
				curStyle = tcell.StyleDefault.Foreground(tcell.ColorLightGrey).Bold(true)
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

	// Определяем цвет для активной панели
	panelColor := tcell.ColorGray
	if a.activePanel == "left" {
		panelColor = tcell.ColorBlue
	} else {
		panelColor = tcell.ColorGreen
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
		style := tcell.StyleDefault.Foreground(tcell.ColorGray)
		// Проверяем, находится ли символ в области панели
		if col >= panelStart && col < panelStart+5 {
			style = style.Foreground(panelColor).Bold(true)
		}
		// Проверяем, находится ли символ в области режима
		if col >= modeStart && col < modeStart+8 {
			// Определяем цвет для активного режима
			color := tcell.ColorGray
			if a.mode == "edit" {
				color = tcell.ColorBlue
			} else {
				color = tcell.ColorGreen
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
	case tcell.KeyCtrlF:
		a.formatWithGofmt()
	case tcell.KeyCtrlG:
		a.formatWithGoimports()
	case tcell.KeyDelete:
		if a.activePanel == "left" {
			a.deleteFile()
		}
	case tcell.KeyTab:
		// Tab не переключает в preview — режим preview отключен в UI.
		// Можно использовать Tab для других целей при желании.
	case tcell.KeyCtrlT:
		a.toggleTerminal() // новый вызов терминала
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
			lines[a.editY] = string(append(append(runes[:a.editX], r), runes[a.editX:]...))
			a.setLines(lines)
			a.editX++
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
