package app

import (
	"reflect"
	"testing"

	"github.com/xy200303/MobBase/internal/project"
)

func TestTestProjectCommandForwardsAndSelectsFlutter(t *testing.T) {
	info := &project.Info{Kind: project.KindFlutter, Root: t.TempDir()}
	program, args, err := testProjectCommand(info, []string{"flutter", "test", "test/a_test.dart"})
	if err != nil || program != "flutter" || !reflect.DeepEqual(args, []string{"test", "test/a_test.dart"}) {
		t.Fatalf("command: %q %#v %v", program, args, err)
	}
}
