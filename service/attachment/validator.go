package attachment

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/setting"
)

// 附件校验层的定义。
//
// 安全目标：即使管理员放开了白名单，上传层也不能接受以下内容：
//   1. SVG（无论扩展名、MIME、魔数是什么）—— 产品需求明确禁用，防止 XSS；
//   2. 声称扩展名与实际魔数不一致的文件（text/html 伪装成 txt 等）；
//   3. 包含空字节 / 路径分隔符 / 控制字符 / 保留字的文件名；
//   4. 超过 setting.TicketAttachmentMaxSize 的文件。
//
// 这一层只关心"是否放行"，具体的存储路径由上层生成（UUID+ext），绝不使用原始文件名。

var (
	ErrAttachmentDisabled       = errors.New("ticket attachment feature is disabled")
	ErrAttachmentSizeExceeded   = errors.New("attachment size exceeded")
	ErrAttachmentExtNotAllowed  = errors.New("attachment extension not allowed")
	ErrAttachmentMimeNotAllowed = errors.New("attachment mime type not allowed")
	ErrAttachmentMimeMismatch   = errors.New("attachment declared type does not match content")
	ErrAttachmentSVGForbidden   = errors.New("svg attachments are forbidden")
	ErrAttachmentEmpty          = errors.New("attachment is empty")
	ErrAttachmentFileName       = errors.New("invalid attachment filename")
)

// sniffSize 取文件开头 512 字节用于魔数嗅探（net/http 规范）。
const sniffSize = 512

// SanitizeFileName 清洗客户端传入的文件名：
//   - 只取 basename，去掉客户端可能夹带的路径；
//   - 删除空字节与控制字符；
//   - 把 Windows 保留字 / 非法字符替换为 "_"；
//   - 截断到 200 个 rune，避免存库/展示时出现超长串；
//   - 空名或仅扩展名时给出兜底 "attachment"。
func SanitizeFileName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\x00", "")
	if !utf8.ValidString(name) {
		return "", ErrAttachmentFileName
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == ".." || name == "" || name == string(filepath.Separator) {
		return "", ErrAttachmentFileName
	}
	// 过滤 Windows 非法字符与控制字符。
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r < 0x20, r == 0x7f:
			// 控制字符直接吞掉
			continue
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteRune('_')
		case unicode.IsPrint(r):
			b.WriteRune(r)
		default:
			// 不可打印的 unicode 以 "_" 兜底
			b.WriteRune('_')
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" {
		return "", ErrAttachmentFileName
	}
	// 截断到 200 rune（按字符而非字节，避免中文被拦腰截断成乱码）。
	const maxRunes = 200
	if utf8.RuneCountInString(cleaned) > maxRunes {
		runes := []rune(cleaned)[:maxRunes]
		cleaned = string(runes)
	}
	return cleaned, nil
}

// NormalizeExt 从文件名拿到小写、无前导点的扩展名。无扩展名时返回空串。
func NormalizeExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	return strings.TrimPrefix(ext, ".")
}

// ValidateSize 根据设置判定文件大小。size <= 0 视为未知，允许通过（后续 io.LimitReader 兜底）。
func ValidateSize(size int64) error {
	if size > 0 && size > setting.TicketAttachmentMaxSize {
		return ErrAttachmentSizeExceeded
	}
	return nil
}

// IsSVGContent 判断文件内容是否为 SVG。
// SVG 的 MIME 通常是 image/svg+xml，但某些客户端会回 text/xml 或 application/xml，
// 所以必须同时看内容：任何以 "<svg" / "<?xml" 开头且随后出现 "<svg" 的都视作 SVG 拒绝。
func IsSVGContent(head []byte) bool {
	trimmed := bytes.TrimLeft(head, " \t\r\n\xef\xbb\xbf") // 去 BOM 和空白
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte("<svg")) {
		return true
	}
	if bytes.HasPrefix(lower, []byte("<?xml")) && bytes.Contains(lower, []byte("<svg")) {
		return true
	}
	return false
}

// DetectMime identifies content from its first 512 bytes, never client metadata.
func DetectMime(head []byte) string {
	mime := http.DetectContentType(head)
	// http.DetectContentType 对纯英文 JSON 也会返回 "text/plain; charset=utf-8"；
	// 这里保留原始结果，由上层的白名单 + 扩展名一致性判定兜底。
	return mime
}

// CheckResult 返回真实可信的 MIME 与扩展名，供存储/DB 写入使用。
type CheckResult struct {
	SafeName     string // 清洗后的展示文件名
	Ext          string // 小写扩展名（无点）
	DetectedMime string // http.DetectContentType 嗅探值，不是客户端声明值
}

// Validate checks content against the extension and configured MIME allowlists.
// The legacy declared-MIME argument is ignored; clients cannot select the stored MIME.
func Validate(fileName string, _ string, size int64, head []byte) (*CheckResult, error) {
	if !setting.TicketAttachmentEnabled {
		return nil, ErrAttachmentDisabled
	}
	if size == 0 && len(head) == 0 {
		return nil, ErrAttachmentEmpty
	}
	if err := ValidateSize(size); err != nil {
		return nil, err
	}

	safeName, err := SanitizeFileName(fileName)
	if err != nil {
		return nil, err
	}
	ext := NormalizeExt(safeName)
	if ext == "" {
		return nil, ErrAttachmentExtNotAllowed
	}
	// 第一重：扩展名白名单。
	if !setting.IsTicketAttachmentExtAllowed(ext) {
		return nil, ErrAttachmentExtNotAllowed
	}
	// 硬性拒绝 svg，即使管理员误加到白名单里。
	if ext == "svg" {
		return nil, ErrAttachmentSVGForbidden
	}

	// File signatures determine the MIME independently of multipart headers.
	truncated := head
	if len(truncated) > sniffSize {
		truncated = truncated[:sniffSize]
	}
	if IsSVGContent(truncated) {
		return nil, ErrAttachmentSVGForbidden
	}
	detected := strings.Split(DetectMime(truncated), ";")[0]
	var expected string
	switch ext {
	case "jpg", "jpeg":
		expected = "image/jpeg"
	case "png", "gif", "webp", "bmp":
		expected = "image/" + ext
	case "pdf":
		expected = "application/pdf"
	case "txt", "log", "md", "csv", "json":
		expected = "text/plain"
	case "xml":
		expected = "text/xml"
		if detected == "text/plain" {
			expected = "text/plain"
		}
	default:
		return nil, ErrAttachmentMimeMismatch
	}
	if detected != expected {
		return nil, ErrAttachmentMimeMismatch
	}
	if !setting.IsTicketAttachmentMimeAllowed(detected) {
		return nil, ErrAttachmentMimeNotAllowed
	}

	return &CheckResult{
		SafeName:     safeName,
		Ext:          ext,
		DetectedMime: detected,
	}, nil
}

// ReadSniff 从 reader 中最多读取 sniffSize 字节用于嗅探，
// 并把已经读出的字节重新拼回，返回一个"内容完整"的新 reader。
// 适用于 multipart 文件：不能 Seek，也不想把整个文件读进内存。
func ReadSniff(r io.Reader) (head []byte, combined io.Reader, err error) {
	buf := make([]byte, sniffSize)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, nil, err
	}
	head = buf[:n]
	combined = io.MultiReader(bytes.NewReader(head), r)
	return head, combined, nil
}
