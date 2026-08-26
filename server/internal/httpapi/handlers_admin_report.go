package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/db"
	"github.com/hkjang/ptium/server/internal/store"
)

// One page an operator can hand to somebody who cannot see their deployment.
//
// A site with no internet cannot open a dashboard for anybody, and a support
// question from one arrives as a sentence: "생성이 안 됩니다". What the version
// is, whether the database is up to date, which settings were changed from what
// this product ships with, whether the designs can draw, what has failed lately
// and how full the disk is — all of it exists in this product and none of it
// could be sent anywhere. This is that, as one answer and as one file.
//
// It never carries a secret: a setting that holds one is reported as configured
// or not, and never as its value.

type reportSetting struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value,omitempty"`
	Configured bool            `json:"configured,omitempty"`
	Sensitive  bool            `json:"sensitive,omitempty"`
}

type deploymentReport struct {
	Version   string             `json:"version"`
	TakenAt   time.Time          `json:"takenAt"`
	Schema    store.SchemaState  `json:"schema"`
	Overview  store.Overview     `json:"overview"`
	Storage   store.StorageUsage `json:"storage"`
	Usage     store.Usage        `json:"usage"`
	Tidy      store.TidyPreview  `json:"tidy"`
	Changed   []reportSetting    `json:"changedSettings"`
	Designs   reportDesigns      `json:"designs"`
	ModelHost string             `json:"modelHost"`
}

type reportDesigns struct {
	Builtin  int    `json:"builtin"`
	Uploaded int    `json:"uploaded"`
	Standard string `json:"standard"`
}

func (s *Server) adminReport(writer http.ResponseWriter, request *http.Request) {
	report, err := s.buildReport(request)
	if err != nil {
		s.internalError(writer, request, "admin_report_failed", err)
		return
	}
	if strings.EqualFold(strings.TrimSpace(request.URL.Query().Get("format")), "md") {
		writer.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		writer.Header().Set("Content-Disposition",
			fmt.Sprintf(`attachment; filename="ptium-%s-%s.md"`, report.Version, report.TakenAt.Format("20060102")))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(writeReport(report)))
		return
	}
	writeData(writer, request, http.StatusOK, report)
}

func (s *Server) buildReport(request *http.Request) (deploymentReport, error) {
	ctx := request.Context()
	report := deploymentReport{Version: s.version, TakenAt: time.Now().UTC()}
	var err error
	if report.Schema, err = s.store.ReadSchemaState(ctx); err != nil {
		return report, err
	}
	if report.Overview, err = s.store.AdminOverview(ctx); err != nil {
		return report, err
	}
	if report.Storage, err = s.store.Storage(ctx, s.assetDir); err != nil {
		return report, err
	}
	if report.Usage, err = s.store.ReadUsage(ctx, 7); err != nil {
		return report, err
	}
	if report.Tidy, err = s.store.ReadTidyPreview(ctx); err != nil {
		return report, err
	}

	// The settings this deployment does not run at what it shipped with. A page
	// of forty values nobody changed hides the two somebody did.
	stored, err := s.settings.ListForAdmin(ctx)
	if err != nil {
		return report, err
	}
	for _, setting := range stored {
		if setting.Sensitive {
			// A secret nobody has set is the shipped state, and listing it as a
			// change fills the report with things nobody did.
			if setting.Configured {
				report.Changed = append(report.Changed, reportSetting{
					Key: setting.Key, Sensitive: true, Configured: true})
			}
			continue
		}
		if shipped, known := db.ShippedSetting(setting.Key); known && sameSetting(setting.Value, shipped) {
			continue
		}
		report.Changed = append(report.Changed, reportSetting{Key: setting.Key, Value: setting.Value})
	}
	sort.Slice(report.Changed, func(one, other int) bool { return report.Changed[one].Key < report.Changed[other].Key })

	standard := ""
	_ = s.settings.Get(ctx, "generation.default_theme", &standard)
	report.Designs.Standard = standard
	designs, _, err := s.store.ListDeploymentTemplates(ctx, store.TemplateFilter{}, standard, 100, 0)
	if err != nil {
		return report, err
	}
	for _, design := range designs {
		if design.Kind == "builtin" {
			report.Designs.Builtin++
		} else {
			report.Designs.Uploaded++
		}
	}
	provider := "fallback"
	_ = s.settings.Get(ctx, "ai.provider", &provider)
	if s.modelConnected(ctx) {
		base := ""
		_ = s.settings.Get(ctx, "ai.base_url", &base)
		report.ModelHost = fmt.Sprintf("%s · %s", provider, base)
	} else {
		report.ModelHost = "내장 생성기 (모델 호스트 없음)"
	}
	return report, nil
}

// writeReport is the same answer as a page somebody can read and send.
func writeReport(report deploymentReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Ptium 배포 점검 · %s\n\n", report.Version)
	fmt.Fprintf(&out, "- 확인 시각: %s\n", report.TakenAt.Format("2006-01-02 15:04 MST"))
	fmt.Fprintf(&out, "- 데이터베이스 마이그레이션: %d개 적용 (최신 %d)\n", report.Schema.Applied, report.Schema.Latest)
	fmt.Fprintf(&out, "- 생성 엔진: %s\n", report.ModelHost)
	fmt.Fprintf(&out, "- 디자인: 내장 %d · 올린 템플릿 %d · 표준 %s\n\n",
		report.Designs.Builtin, report.Designs.Uploaded, orNone(report.Designs.Standard))

	fmt.Fprintf(&out, "## 지금\n\n")
	fmt.Fprintf(&out, "- 사용자 %d · 덱 %d (완성 %d · 휴지통 %d)\n",
		report.Overview.Users, report.Overview.Presentations, report.Overview.CompletedDecks, report.Overview.DeletedDecks)
	fmt.Fprintf(&out, "- 대기·작성 중 %d · 가장 오래 기다린 %d초 · 가장 조용한 작성 %d초\n",
		report.Overview.QueuedGenerations, report.Overview.OldestQueuedSeconds, report.Overview.QuietestGenerationSeconds)
	fmt.Fprintf(&out, "- 24시간 실패 %d · 열린 오류 %d\n", report.Overview.FailedLastDay, report.Overview.OpenIncidents)
	fmt.Fprintf(&out, "- 데이터베이스 %s · 이미지 %s\n\n",
		bytesWord(report.Storage.DatabaseBytes), bytesWord(report.Storage.AssetsInRows+report.Storage.AssetsInVolume))

	fmt.Fprintf(&out, "## 최근 7일\n\n")
	fmt.Fprintf(&out, "- 만든 덱 %d · 실패 %d · 시간이 기록된 생성 %d\n", report.Usage.Generated, report.Usage.Failed, report.Usage.Timed)
	for _, day := range report.Usage.Days {
		fmt.Fprintf(&out, "  - %s 생성 %d · 실패 %d · 중앙 %.2f초 · 가장 오래 %.2f초\n",
			day.Day, day.Generated, day.Failed, day.MedianSeconds, day.SlowestSeconds)
	}
	if len(report.Usage.Failures) > 0 {
		fmt.Fprintf(&out, "\n### 실패한 이유\n\n")
		for _, reason := range report.Usage.Failures {
			fmt.Fprintf(&out, "- %d회 · %s\n", reason.Count, reason.Name)
		}
	}

	fmt.Fprintf(&out, "\n## 출고값과 다른 설정\n\n")
	if len(report.Changed) == 0 {
		fmt.Fprintf(&out, "- 없습니다 (출고된 그대로입니다)\n")
	}
	for _, setting := range report.Changed {
		if setting.Sensitive {
			fmt.Fprintf(&out, "- %s: %s (값은 적지 않습니다)\n", setting.Key, configuredWord(setting.Configured))
			continue
		}
		fmt.Fprintf(&out, "- %s: %s\n", setting.Key, strings.TrimSpace(string(setting.Value)))
	}

	fmt.Fprintf(&out, "\n## 쌓인 것 (아무것도 지우지 않았습니다)\n\n")
	for _, item := range report.Tidy.Items {
		line := fmt.Sprintf("- %s: %d개", tidyWord(item.Kind), item.Count)
		if item.Bytes > 0 {
			line += " · " + bytesWord(item.Bytes)
		}
		if item.Oldest != "" {
			line += " · 가장 오래된 것 " + item.Oldest
		}
		fmt.Fprintln(&out, line)
	}
	fmt.Fprintf(&out, "\n이 파일에는 비밀 값이 들어 있지 않습니다.\n")
	return out.String()
}

func orNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "지정 없음"
	}
	return value
}

func configuredWord(configured bool) string {
	if configured {
		return "설정됨"
	}
	return "설정 안 됨"
}

func bytesWord(bytes int64) string {
	switch {
	case bytes <= 0:
		return "0"
	case bytes < 1<<20:
		return fmt.Sprintf("%dKB", bytes>>10)
	case bytes < 1<<30:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1<<20))
	}
	return fmt.Sprintf("%.2fGB", float64(bytes)/(1<<30))
}

// sameSetting compares two stored values by what they mean rather than how they
// are written: ["ptium-admin","admin"] and ["ptium-admin", "admin"] are the same
// setting, and a report that calls the second one a change is telling an
// operator somebody edited something nobody edited.
func sameSetting(stored json.RawMessage, shipped string) bool {
	var one, other any
	if json.Unmarshal(stored, &one) != nil || json.Unmarshal([]byte(shipped), &other) != nil {
		return strings.TrimSpace(string(stored)) == strings.TrimSpace(shipped)
	}
	left, err := json.Marshal(one)
	if err != nil {
		return false
	}
	right, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

// tidyWord is what a pile is called in the language this report is read in.
// A Korean page listing "unusedImagesOverAMonth" is asking its reader to know
// this product's field names.
func tidyWord(kind string) string {
	switch kind {
	case "trashed":
		return "휴지통에 있는 덱"
	case "failedOldDecks":
		return "30일 넘게 실패로 남은 덱"
	case "untouchedDrafts":
		return "90일 넘게 손대지 않은 초안"
	case "expiredLinks":
		return "기한이 지난 공유 링크"
	case "unusedImages":
		return "어느 덱도 쓰지 않는 이미지"
	case "unusedImagesOverAMonth":
		return "한 달 넘게 쓰이지 않은 이미지"
	}
	return kind
}
