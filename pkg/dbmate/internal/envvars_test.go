package internal_test

import (
	"testing"

	"github.com/amacneil/dbmate/v2/pkg/dbmate/internal"

	"github.com/stretchr/testify/require"
)

func TestResolveRefs(t *testing.T) {
	parseUp := `create role '{{ .THE_ROLE }}' login password '{{ .THE_PASSWORD }}';`
	parsedUpOptsEnvVars := []string{
		"THE_ROLE",
		"THE_PASSWORD",
	}
	envMap := map[string]string{
		"THE_ROLE":     "Barney",
		"THE_PASSWORD": "Betty",
	}

	resolved, err := internal.ResolveRefs(parseUp, parsedUpOptsEnvVars, envMap)

	require.NoError(t, err)
	require.Equal(t, "create role 'Barney' login password 'Betty';", resolved)
}

func TestResolveRefsUsingDefaults(t *testing.T) {
	parseUp := `create role '{{ or (index . "THE_ROLE") "Fred" }}' login password '{{ .THE_PASSWORD }}';`
	parsedUpOptsEnvVars := []string{
		"THE_ROLE",
		"THE_PASSWORD",
	}
	envMap := map[string]string{
		"THE_PASSWORD": "Wilma",
	}

	resolved, err := internal.ResolveRefs(parseUp, parsedUpOptsEnvVars, envMap)

	require.NoError(t, err)
	require.Equal(t, "create role 'Fred' login password 'Wilma';", resolved)
}

func TestResolveRefsIgnoringDefaults(t *testing.T) {
	parseUp := `create role '{{ or (index . "THE_ROLE") "Fred" }}' login password '{{ .THE_PASSWORD }}';`
	parsedUpOptsEnvVars := []string{
		"THE_ROLE",
		"THE_PASSWORD",
	}
	envMap := map[string]string{
		"THE_ROLE":     "Dino",
		"THE_PASSWORD": "Wilma",
	}

	resolved, err := internal.ResolveRefs(parseUp, parsedUpOptsEnvVars, envMap)

	require.NoError(t, err)
	require.Equal(t, "create role 'Dino' login password 'Wilma';", resolved)
}

func TestResolveRefsErroringOnMissingVar(t *testing.T) {
	parseUp := `create role '{{ or (index . "THE_ROLE") "Fred" }}' login password '{{ .THE_PASSWORD }}';`
	parsedUpOptsEnvVars := []string{
		"THE_ROLE",
		"THE_PASSWORD",
	}
	envMap := map[string]string{
		"THE_ROLE": "Dino",
	}

	resolved, err := internal.ResolveRefs(parseUp, parsedUpOptsEnvVars, envMap)

	require.Error(t, err)
	require.Equal(t, "", resolved)
}

func TestResolveWithSqlInjection(t *testing.T) {
	parseUp := `create role '{{ .THE_ROLE }}' login password '{{ .THE_PASSWORD }}';`
	parsedUpOptsEnvVars := []string{
		"THE_ROLE",
		"THE_PASSWORD",
	}
	envMap := map[string]string{
		"THE_ROLE":     "Slate'; drop table SALARY; create role 'Barney",
		"THE_PASSWORD": "Betty",
	}

	resolved, err := internal.ResolveRefs(parseUp, parsedUpOptsEnvVars, envMap)

	require.NoError(t, err)
	// sql injection is not prevented here
	require.Equal(t, "create role 'Slate'; drop table SALARY; create role 'Barney' login password 'Betty';", resolved)
}

func TestResolveMitigatingSqlInjection(t *testing.T) {
	parseUp := `create role '{{ js .THE_ROLE }}' login password '{{ js .THE_PASSWORD }}';`
	parsedUpOptsEnvVars := []string{
		"THE_ROLE",
		"THE_PASSWORD",
	}
	envMap := map[string]string{
		// simulating naive SQL injection attempt
		"THE_ROLE":     "Slate'; drop table SALARY; create role 'Barney",
		"THE_PASSWORD": "Betty",
	}

	resolved, err := internal.ResolveRefs(parseUp, parsedUpOptsEnvVars, envMap)

	require.NoError(t, err)
	// sql injection mitigated
	require.Equal(t, "create role 'Slate\\'; drop table SALARY; create role \\'Barney' login password 'Betty';", resolved)
}
