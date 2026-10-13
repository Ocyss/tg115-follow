package parse

import (
	"testing"

	"github.com/ocyss/tg115-follow/internal/store"
)

const subscribeText = "✅ 订阅成功\n\n📺 资源订阅\n\n择日飞升 (2026)\n类型: 剧集\n\n后续有新资源或更新时，会通过这里通知你。\n\n👉 查看资源详情"

var subscribeEntities = []store.Entity{
	{Type: "bold", Offset: 2, Length: 4},
	{Type: "bold", Offset: 11, Length: 4},
	{Type: "bold", Offset: 17, Length: 4},
	{Type: "bold", Offset: 33, Length: 2},
	{Type: "text_url", Offset: 62, Length: 6, URL: "https://re0.me/tv/a23a1f7c-5270-438e-ba16-db61ebd41444"},
}

const updateText = "您订阅的「吞噬星空」已更新\n\n[剧集·115] 吞噬星空 (2020)\n244  已更  1-244不缺集  已整理标准命名  长期更新  防止违规  未添加季文件夹。\n\n简介：RR病毒席卷全球，受感染的动物异变为恐怖怪兽，人类文明几近毁灭。在绝境中，人类筑起高墙，建立基地市作...\n\n分享：无言以对\nTMDB: 101172\n大小：280.83G\n字幕：内封简中\n\n直达链接\n\n标签：#吞噬星空 #动画 #Sci_Fi&Fantasy #动作冒险\n\n━━━━━━━━━━━━━━\n🎬 GVS 影视 API 商城\n腾讯视频 · 优酷 · 红果短剧\n\n👉 进入商城"

var updateEntities = []store.Entity{
	{Type: "bold", Offset: 15, Length: 20},
	{Type: "bold", Offset: 36, Length: 48},
	{Type: "text_url", Offset: 147, Length: 4, URL: "https://re0.me/user/17667"},
	{Type: "text_url", Offset: 158, Length: 6, URL: "https://www.themoviedb.org/tv/101172"},
	{Type: "text_url", Offset: 185, Length: 4, URL: "https://re0.me/resource/115/9ad89a82e4f544a18239e4c823cf5593"},
	{Type: "text_url", Offset: 280, Length: 4, URL: "https://gvs.videohack.shop/shop"},
}

func TestParseSubscribe(t *testing.T) {
	r := Parse(subscribeText, subscribeEntities)
	if r.Type != store.MessageTypeSubscribe {
		t.Fatalf("type = %q", r.Type)
	}
	s := r.Subscribe
	if s.Title != "择日飞升 (2026)" {
		t.Errorf("title = %q", s.Title)
	}
	if s.MediaType != store.MediaTypeSeries {
		t.Errorf("media = %q", s.MediaType)
	}
	if s.DetailURL != "https://re0.me/tv/a23a1f7c-5270-438e-ba16-db61ebd41444" {
		t.Errorf("detail = %q", s.DetailURL)
	}
}

func TestParseUpdate(t *testing.T) {
	r := Parse(updateText, updateEntities)
	if r.Type != store.MessageTypeUpdate {
		t.Fatalf("type = %q", r.Type)
	}
	u := r.Update
	if u.Title != "吞噬星空 (2020)" {
		t.Errorf("title = %q", u.Title)
	}
	if u.MediaType != store.MediaTypeSeries {
		t.Errorf("media = %q", u.MediaType)
	}
	if u.DiskType != "115" {
		t.Errorf("disk = %q", u.DiskType)
	}
	if u.ShareName != "无言以对" || u.ShareURL != "https://re0.me/user/17667" {
		t.Errorf("share = %q %q", u.ShareName, u.ShareURL)
	}
	if u.ResourceURL != "https://re0.me/resource/115/9ad89a82e4f544a18239e4c823cf5593" {
		t.Errorf("resource = %q", u.ResourceURL)
	}
	if u.TMDBID != 101172 {
		t.Errorf("tmdb = %d", u.TMDBID)
	}
	if u.Size != "280.83G" || u.Subtitle != "内封简中" {
		t.Errorf("size/subtitle = %q %q", u.Size, u.Subtitle)
	}
}

func TestParseUnknown(t *testing.T) {
	r := Parse("随便一条消息", nil)
	if r.Type != store.MessageTypeUnknown {
		t.Fatalf("type = %q", r.Type)
	}
}

func TestParseUpdateSizeSubtitle(t *testing.T) {
	// Size present, subtitle with comma.
	u1 := Parse("您订阅的「X」已更新\n\n[电影·夸克] 蜘蛛侠：崭新之日 (2026)\n\n分享：年糕\nTMDB: 969681\n大小：9G/集\n字幕：内封简中, 简英双语\n\n直达链接", []store.Entity{
		{Type: "bold", Offset: 15, Length: 24},
	})
	if u1.Update.Size != "9G/集" || u1.Update.Subtitle != "内封简中, 简英双语" {
		t.Errorf("size/subtitle = %q %q", u1.Update.Size, u1.Update.Subtitle)
	}

	// Empty size must not swallow the next line.
	u2 := Parse("您订阅的「X」已更新\n\n[电影·115] 蜘蛛侠：崭新之日 (2026)\n\n分享：Superxiao\nTMDB: 969681\n大小：\n\n直达链接", nil)
	if u2.Update.Size != "" {
		t.Errorf("size = %q, want empty", u2.Update.Size)
	}
}
