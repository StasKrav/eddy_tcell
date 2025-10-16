package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// Цветовые константы
const (
	ColorGrey = tcell.ColorGrey // Цвет рамки левой панели
	ColorRed  = tcell.ColorRed  // Цвет заголовка левой панели и фона выделенного элемента
	//  ColorBlue      = tcell.ColorYellow    // Цвет директорий и ключевых слов Go
	//  ColorGreen     = tcell.ColorGreen     // Цвет заголовка правой панели и комментариев
	ColorYellow    = tcell.ColorYellow    // Цвет строк в коде / строки
	ColorWhite     = tcell.ColorWhite     // Базовый цвет текста
	ColorAqua      = tcell.ColorAqua      // Цвет типов (если нужно)
	ColorFuchsia   = tcell.ColorFuchsia   // Цвет чисел (если нужно)
	ColorGray      = tcell.ColorGray      // Цвет статусной строки и списков
	ColorOlive     = tcell.ColorOlive     // Цвет заголовков третьего уровня в предпросмотре
	ColorLightBlue = tcell.ColorLightBlue // Цвет ссылок в предпросмотре
	ColorLightGrey = tcell.ColorLightGrey // Цвет маркеров списков в предпросмотре
	ColorBlack     = tcell.ColorBlack     // Цвет фона строк и курсора
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
}

// Тип токена для подсветки (остался если понадобится)
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


ИНДИКАТОРЫ:


- в заголовке редактора означает, что файл был изменен, но еще не сохранен

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

	// Рисуем правую панель (редактор/предпросмотр)
	a.drawEditor()

	// Рисуем статусную строку
	a.drawStatus()

	a.screen.Show()

}

// Отрисовка списка файлов
func (a *App) drawFileList() {
	// Рамка левой панели
	for y := 0; y < a.height-3; y++ {
		a.screen.SetContent(a.leftWidth, y, '│', nil, tcell.StyleDefault.Foreground(ColorGrey))
	}

	// Заголовок
	title := "Files"
	col := 0
	for _, r := range title {
		w := runewidth.RuneWidth(r)
		if col >= a.leftWidth-2 {
			break
		}
		a.screen.SetContent(col+1, 0, r, nil, tcell.StyleDefault.Foreground(ColorRed).Bold(true))
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
			style = style.Background(ColorRed).Foreground(ColorBlack)
		}

		// Имя файла
		name := file.name
		if file.isDir {
			name += "/"
			style = style.Foreground(ColorBlue)
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
	for _, r := range title {
		w := runewidth.RuneWidth(r)
		if col >= maxTitleCols {
			break
		}
		a.screen.SetContent(a.leftWidth+1+col, 0, r, nil, tcell.StyleDefault.Foreground(ColorGreen).Bold(true))
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
					a.screen.SetContent(cursorX, cursorY, ' ', nil, tcell.StyleDefault.Background(ColorWhite).Foreground(ColorBlack))
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
				style = style.Background(ColorWhite).Foreground(ColorBlack)
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
				a.screen.SetContent(cursorX, y, ' ', nil, tcell.StyleDefault.Background(ColorWhite).Foreground(ColorBlack)) // рисуем инвертированный пробел
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
		baseStyle := tcell.StyleDefault.Foreground(ColorWhite)

		// decide line-level style and possibly trim prefixes
		if inCodeBlock {
			baseStyle = tcell.StyleDefault.Foreground(ColorYellow).Background(ColorBlack)
		} else if strings.HasPrefix(trim, "# ") {
			trim = strings.TrimPrefix(trim, "# ")
			baseStyle = tcell.StyleDefault.Foreground(ColorGreen).Bold(true)
		} else if strings.HasPrefix(trim, "## ") {
			trim = strings.TrimPrefix(trim, "## ")
			baseStyle = tcell.StyleDefault.Foreground(ColorGreen).Bold(true)
		} else if strings.HasPrefix(trim, "### ") {
			trim = strings.TrimPrefix(trim, "### ")
			baseStyle = tcell.StyleDefault.Foreground(ColorOlive).Bold(true)
		} else if strings.HasPrefix(strings.TrimLeft(trim, " "), "> ") {
			// blockquote, keep indentation
			// remove one leading '>' if present after spaces
			idx := strings.Index(trim, "> ")
			if idx >= 0 {
				trim = strings.TrimSpace(trim[idx+2:])
			}
			baseStyle = tcell.StyleDefault.Foreground(ColorBlue).Italic(true)
		} else if listRe.MatchString(trim) {
			// don't strip marker completely; will color marker when rendering
			baseStyle = tcell.StyleDefault.Foreground(ColorGray)
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
							linkStyle := baseStyle.Foreground(ColorLightBlue).Underline(true)
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
				curStyle = tcell.StyleDefault.Foreground(ColorBlack).Background(ColorWhite)
			} else if inEmphasis {
				curStyle = curStyle.Bold(true)
			}

			// special: color list marker differently if at line start
			// Учитываем смещение при горизонтальной прокрутке
			if (r == '-' || r == '+' || r == '*') && idx == 0 && listRe.MatchString(string(runes)) {
				curStyle = tcell.StyleDefault.Foreground(ColorLightGrey).Bold(true)
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
	panelColor := ColorGray
	if a.activePanel == "left" {
		panelColor = ColorBlue
	} else {
		panelColor = ColorGreen
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
		style := tcell.StyleDefault.Foreground(ColorGray)
		// Проверяем, находится ли символ в области панели
		if col >= panelStart && col < panelStart+5 {
			style = style.Foreground(panelColor).Bold(true)
		}
		// Проверяем, находится ли символ в области режима
		if col >= modeStart && col < modeStart+8 {
			// Определяем цвет для активного режима
			color := ColorGray
			if a.mode == "edit" {
				color = ColorBlue
			} else {
				color = ColorGreen
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
