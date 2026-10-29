package store

import (
	"errors"
	"fmt"
	"testing"
)

func newMail(id string, recipients ...string) NewMail {
	return NewMail{
		ID: id, From: "s@other.com", To: recipients, Subject: "s " + id, Text: "t",
		ReceivedAt: 100, Recipients: recipients,
	}
}

func TestInsertEmailIgnoresDuplicate(t *testing.T) {
	st := newTestStore(t)
	acct, _ := st.CreateAccount("a@example.com", "h", "", 1)

	m := newMail("m1", "a@example.com")
	m.Attachments = []Attachment{{ID: "att", Filename: "f", ContentType: "x/y"}}
	if err := st.InsertEmail(m); err != nil {
		t.Fatal(err)
	}
	m.Subject = "changed"
	if err := st.InsertEmail(m); err != nil {
		t.Fatalf("重复写入应被忽略, err = %v", err)
	}

	got, err := st.GetAccountMail(acct.ID, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "s m1" || len(got.Attachments) != 1 {
		t.Fatalf("重复写入不应覆盖或重复附件: %+v", got)
	}
}

func TestExistingEmailIDsAcrossChunkBoundary(t *testing.T) {
	st := newTestStore(t)
	for _, id := range []string{"e0", "e700", "e1199"} {
		if err := st.InsertEmail(newMail(id, "a@example.com")); err != nil {
			t.Fatal(err)
		}
	}
	all := make([]string, 1200)
	for i := range all {
		all[i] = fmt.Sprintf("e%d", i)
	}
	found, err := st.ExistingEmailIDs(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 3 || !found["e0"] || !found["e700"] || !found["e1199"] {
		t.Fatalf("found = %v", found)
	}
	if found, _ := st.ExistingEmailIDs(nil); len(found) != 0 {
		t.Fatal("空输入应返回空集合")
	}
}

func TestMailVisibilityFollowsAccountAddress(t *testing.T) {
	st := newTestStore(t)
	st.InsertEmail(newMail("m1", "a@example.com"))

	if _, total, _ := st.ListUnroutedMails(10, 0); total != 1 {
		t.Fatal("账号不存在时应为未匹配")
	}
	acct, _ := st.CreateAccount("a@example.com", "h", "", 1)
	if _, total, _ := st.ListAccountMails(acct.ID, 10, 0); total != 1 {
		t.Fatal("创建同地址账号后应可见")
	}
	if _, total, _ := st.ListUnroutedMails(10, 0); total != 0 {
		t.Fatal("已匹配后不应再算未匹配")
	}
	if err := st.DeleteAccount(acct.ID); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := st.ListUnroutedMails(10, 0); total != 1 {
		t.Fatal("账号删除后邮件应回到未匹配")
	}
}

func TestListMailsOrderAndPaging(t *testing.T) {
	st := newTestStore(t)
	acct, _ := st.CreateAccount("a@example.com", "h", "", 1)
	for i, id := range []string{"old", "mid", "new"} {
		m := newMail(id, "a@example.com")
		m.ReceivedAt = int64(100 + i)
		st.InsertEmail(m)
	}

	mails, total, _ := st.ListAccountMails(acct.ID, 2, 0)
	if total != 3 || len(mails) != 2 || mails[0].ID != "new" || mails[1].ID != "mid" {
		t.Fatalf("第一页 = %+v (total %d)", mails, total)
	}
	mails, _, _ = st.ListAccountMails(acct.ID, 2, 2)
	if len(mails) != 1 || mails[0].ID != "old" {
		t.Fatalf("第二页 = %+v", mails)
	}
}

func TestMarkReadIsPerAccountAndCascades(t *testing.T) {
	st := newTestStore(t)
	a, _ := st.CreateAccount("a@example.com", "h", "", 1)
	b, _ := st.CreateAccount("b@example.com", "h", "", 1)
	st.InsertEmail(newMail("m1", "a@example.com", "b@example.com"))

	if err := st.MarkRead(a.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkRead(a.ID, "m1"); err != nil {
		t.Fatalf("重复标记应幂等: %v", err)
	}
	ma, _, _ := st.ListAccountMails(a.ID, 10, 0)
	mb, _, _ := st.ListAccountMails(b.ID, 10, 0)
	if !ma[0].IsRead || mb[0].IsRead {
		t.Fatalf("已读状态应按账号隔离: a=%v b=%v", ma[0].IsRead, mb[0].IsRead)
	}
}

func TestGetMailRejectsForeignAccountAndMissing(t *testing.T) {
	st := newTestStore(t)
	a, _ := st.CreateAccount("a@example.com", "h", "", 1)
	b, _ := st.CreateAccount("b@example.com", "h", "", 1)
	st.InsertEmail(newMail("m1", "a@example.com"))

	if _, err := st.GetAccountMail(b.ID, "m1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("他人邮件 err = %v", err)
	}
	if _, err := st.GetAccountMail(a.ID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在邮件 err = %v", err)
	}
	if _, err := st.GetUnroutedMail("m1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("已分发邮件不应出现在未匹配 err = %v", err)
	}
	if _, err := st.GetAttachment("m1", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("附件 err = %v", err)
	}
}
