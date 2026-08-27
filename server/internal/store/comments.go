package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
)

// A link lets someone look at a deck. Looking is half of a review: the other
// half is saying what is wrong with slide 4, and until there was somewhere to
// say it that came back as an email the author had to hold beside the deck.
//
// A comment is attached to the slide it is about, by id, so it stays on that
// slide when the deck is reordered. It says who left it in the name they typed:
// the person following the link has no account here, and asking them to make
// one to say "the number on slide 4 is out of date" is how a review does not
// happen.

// MaximumComments is how many comments one deck will hold. A review is a
// conversation, not a queue; past this something is wrong and a public endpoint
// should stop writing rows.
const MaximumComments = 500

// CommentInput is a comment as it arrives.
type CommentInput struct {
	SlideID string
	ShareID string
	Author  string
	Body    string
	// ParentID is the remark this one answers. A reply belongs under its remark
	// and to the same slide, whatever slide the answering window happens to be
	// showing; a reply to a reply is a tree, and a review is a conversation, so
	// answering an answer joins the same thread.
	ParentID string
}

// ErrTooManyComments says this deck has had all the comments it will take.
// Said in Korean because a shared link hands it straight to a reader outside
// the workspace, on a page that is Korean throughout.
var ErrTooManyComments = errors.New("이 덱에는 더 이상 의견을 남길 수 없습니다.")

// slide_id comes back as text: a uuid column scans into a driver type, and a
// comment that lost the slide it was about is a comment about nothing.
const commentColumns = `id,presentation_id,coalesce(slide_id::text,''),author_name,body,coalesce(parent_id::text,''),resolved_at,created_at`

// AddComment records one remark about one slide.
func (s *Store) AddComment(ctx context.Context, presentationID string, in CommentInput) (model.Comment, error) {
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return model.Comment{}, errors.New("a comment needs something to say")
	}
	if utf8.RuneCountInString(body) > 4000 {
		return model.Comment{}, errors.New("that comment is too long")
	}
	author := strings.TrimSpace(in.Author)
	if utf8.RuneCountInString(author) > 80 {
		return model.Comment{}, errors.New("that name is too long")
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM slide_comments WHERE presentation_id=$1`,
		presentationID).Scan(&count); err != nil {
		return model.Comment{}, err
	}
	if count >= MaximumComments {
		return model.Comment{}, ErrTooManyComments
	}
	parent, slide := strings.TrimSpace(in.ParentID), in.SlideID
	if parent != "" {
		var root, onSlide string
		err := s.Pool.QueryRow(ctx, `SELECT coalesce(parent_id::text,id::text),coalesce(slide_id::text,'')
			FROM slide_comments WHERE id=$1 AND presentation_id=$2`, parent, presentationID).Scan(&root, &onSlide)
		if err != nil {
			return model.Comment{}, errors.New("that comment is not on this deck")
		}
		// Answering an answer joins the thread rather than starting a branch,
		// and a reply sits on the slide its remark is about.
		parent, slide = root, onSlide
	}
	row := s.Pool.QueryRow(ctx, `INSERT INTO slide_comments(presentation_id,slide_id,share_id,author_name,body,parent_id)
		VALUES($1,nullif($2,'')::uuid,nullif($3,'')::uuid,$4,$5,nullif($6,'')::uuid) RETURNING `+commentColumns,
		presentationID, slide, in.ShareID, author, body, parent)
	return scanComment(row)
}

// Comments are every remark left on one deck, oldest first, so a conversation
// reads in the order it happened.
func (s *Store) Comments(ctx context.Context, presentationID string) ([]model.Comment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+commentColumns+` FROM slide_comments
		WHERE presentation_id=$1 ORDER BY created_at LIMIT $2`, presentationID, MaximumComments)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := make([]model.Comment, 0, 16)
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

// OwnerComments is the same list, for the person who owns the deck.
func (s *Store) OwnerComments(ctx context.Context, presentationID, ownerID string) ([]model.Comment, error) {
	if _, err := s.GetPresentation(ctx, presentationID, ownerID, false); err != nil {
		return nil, err
	}
	return s.Comments(ctx, presentationID)
}

// ResolveComment marks a remark dealt with, or puts it back. Resolving belongs
// to the thread, so answering a reply's resolution is not a separate question:
// the remark and everything under it settle together.
func (s *Store) ResolveComment(ctx context.Context, presentationID, ownerID, commentID string, resolved bool) error {
	if _, err := s.GetPresentation(ctx, presentationID, ownerID, false); err != nil {
		return err
	}
	var when *time.Time
	if resolved {
		now := time.Now()
		when = &now
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE slide_comments SET resolved_at=$1
		WHERE presentation_id=$3 AND (id=$2 OR parent_id=$2)`, when, commentID, presentationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteComment removes a remark. The owner decides what stays on their deck.
func (s *Store) DeleteComment(ctx context.Context, presentationID, ownerID, commentID string) error {
	if _, err := s.GetPresentation(ctx, presentationID, ownerID, false); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM slide_comments WHERE id=$1 AND presentation_id=$2`,
		commentID, presentationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanComment(row shareRow) (model.Comment, error) {
	var comment model.Comment
	var resolved *time.Time
	if err := row.Scan(&comment.ID, &comment.PresentationID, &comment.SlideID, &comment.Author,
		&comment.Body, &comment.ParentID, &resolved, &comment.CreatedAt); err != nil {
		return model.Comment{}, err
	}
	comment.ResolvedAt = resolved
	return comment, nil
}
