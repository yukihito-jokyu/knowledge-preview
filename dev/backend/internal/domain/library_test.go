package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeListQuery(t *testing.T) {
	q, err := NormalizeListQuery(ListQuery{Page: 1})
	require.NoError(t, err)
	require.Equal(t, DefaultPageSize, q.PageSize)

	_, err = NormalizeListQuery(ListQuery{Page: 1, Query: strings.Repeat("あ", MaxSearchRunes+1)})
	require.ErrorIs(t, err, ErrBadRequest)
	_, err = NormalizeListQuery(ListQuery{Page: 1, PageSize: MaxPageSize + 1})
	require.ErrorIs(t, err, ErrBadRequest)
	_, err = NormalizeListQuery(ListQuery{Page: 1, Tags: []string{""}})
	require.ErrorIs(t, err, ErrBadRequest)
}

func TestNormalizeListQueryOffsetBoundaries(t *testing.T) {
	maxInt := int(^uint(0) >> 1)

	const pageSize = 100

	for _, test := range []struct {
		name     string
		page     int
		pageSize int
		wantErr  bool
	}{
		{name: "first page with one item", page: 1, pageSize: 1},
		{name: "second page with one item", page: 2, pageSize: 1},
		{name: "max page with one item", page: maxInt, pageSize: 1},
		{name: "max safe page with max page size", page: maxInt/pageSize + 1, pageSize: pageSize},
		{name: "page after max safe offset", page: maxInt/pageSize + 2, pageSize: pageSize, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeListQuery(ListQuery{Page: test.page, PageSize: test.pageSize})
			if test.wantErr {
				require.ErrorIs(t, err, ErrBadRequest)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestValidateFolderNameTrimsAndLimits(t *testing.T) {
	name, err := ValidateFolderName("  親フォルダ  ")
	require.NoError(t, err)
	require.Equal(t, "親フォルダ", name)

	_, err = ValidateFolderName(strings.Repeat("x", MaxFolderRunes+1))
	require.ErrorIs(t, err, ErrValidation)
}
