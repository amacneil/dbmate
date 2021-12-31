package dbutil

import (
	"reflect"
	"testing"
)

func TestSortByDate(t *testing.T) {
	tests := []struct {
		name          string
		filenameInput []string
		wantSorted    []string
		wantErr       bool
	}{
		{
			name: "Old format",
			filenameInput: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"20211231054630_create_user_role_table.sql",
				"20211231071240_create_tenants_table.sql",
			},
			wantSorted: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"20211231054630_create_user_role_table.sql",
				"20211231071240_create_tenants_table.sql",
			},
			wantErr: false,
		},
		{
			name: "Old format unordered",
			filenameInput: []string{
				"20211231071240_create_tenants_table.sql",
				"20211231054537_create_roles_table.sql",
				"20211231054630_create_user_role_table.sql",
				"20211231054532_create_users_table.sql",
			},
			wantSorted: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"20211231054630_create_user_role_table.sql",
				"20211231071240_create_tenants_table.sql",
			},
			wantErr: false,
		},
		{
			name: "Mixed old and new format",
			filenameInput: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"20211231071240_create_tenants_table.sql",
			},
			wantSorted: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"20211231071240_create_tenants_table.sql",
			},
			wantErr: false,
		},
		{
			name: "Mixed old and new format unordered",
			filenameInput: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"20211231071240_create_tenants_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
			},
			wantSorted: []string{
				"20211231054532_create_users_table.sql",
				"20211231054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"20211231071240_create_tenants_table.sql",
			},
			wantErr: false,
		},
		{
			name: "All new format",
			filenameInput: []string{
				"2021_12_31_054532_create_users_table.sql",
				"2021_12_31_054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"2021_12_31_071240_create_tenants_table.sql",
			},
			wantSorted: []string{
				"2021_12_31_054532_create_users_table.sql",
				"2021_12_31_054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"2021_12_31_071240_create_tenants_table.sql",
			},
			wantErr: false,
		},
		{
			name: "All new format unordered",
			filenameInput: []string{
				"2021_12_31_071240_create_tenants_table.sql",
				"2021_12_31_054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"2021_12_31_054532_create_users_table.sql",
			},
			wantSorted: []string{
				"2021_12_31_054532_create_users_table.sql",
				"2021_12_31_054537_create_roles_table.sql",
				"2021_12_31_054630_create_user_role_table.sql",
				"2021_12_31_071240_create_tenants_table.sql",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSorted, err := SortByDate(tt.filenameInput)
			if (err != nil) != tt.wantErr {
				t.Errorf("SortByDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotSorted, tt.wantSorted) {
				t.Errorf("SortByDate() gotSorted = %v, want %v", gotSorted, tt.wantSorted)
			}
		})
	}
}
