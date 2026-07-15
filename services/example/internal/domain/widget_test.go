// Test Budget: 2 distinct behaviors × 2 = 4 max unit tests
// Actual: 4
//
// Behavior 1: Publish — draft transitions to published; non-draft is rejected
// Behavior 2: Archive — published transitions to archived; non-published is rejected
package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Behavior 1: Publish
// ---------------------------------------------------------------------------

func TestWidget_Publish_Draft_TransitionsToPublished(t *testing.T) {
	w := NewWidget("tenant-001", "hero banner")

	require.NoError(t, w.Publish())
	assert.Equal(t, WidgetStatusPublished, w.Status)
}

func TestWidget_Publish_AlreadyPublished_ReturnsErrNotPublishable(t *testing.T) {
	w := NewWidget("tenant-001", "hero banner")
	require.NoError(t, w.Publish())

	err := w.Publish()

	require.ErrorIs(t, err, ErrNotPublishable)
	assert.Equal(t, WidgetStatusPublished, w.Status, "status must not change on a rejected transition")
}

// ---------------------------------------------------------------------------
// Behavior 2: Archive
// ---------------------------------------------------------------------------

func TestWidget_Archive_Published_TransitionsToArchived(t *testing.T) {
	w := NewWidget("tenant-001", "hero banner")
	require.NoError(t, w.Publish())

	require.NoError(t, w.Archive())
	assert.Equal(t, WidgetStatusArchived, w.Status)
}

func TestWidget_Archive_Draft_ReturnsErrNotArchivable(t *testing.T) {
	w := NewWidget("tenant-001", "hero banner")

	err := w.Archive()

	require.ErrorIs(t, err, ErrNotArchivable)
	assert.Equal(t, WidgetStatusDraft, w.Status, "status must not change on a rejected transition")
}
