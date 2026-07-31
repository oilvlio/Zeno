package api

import "testing"

func TestAdminOptionalNumberUnmarshal(t *testing.T) {
	t.Parallel()

	floatValue := adminOptionalFloat{}
	if err := floatValue.UnmarshalJSON([]byte("1.25")); err != nil {
		t.Fatal(err)
	}
	if !floatValue.Set || !floatValue.Valid || floatValue.Value != 1.25 {
		t.Fatalf("float value = %+v", floatValue)
	}
	if err := floatValue.UnmarshalJSON([]byte(" null \n")); err != nil {
		t.Fatal(err)
	}
	if !floatValue.Set || floatValue.Valid || floatValue.Value != 0 {
		t.Fatalf("null float value = %+v", floatValue)
	}
	floatValue = adminOptionalFloat{Valid: true, Value: 9.5}
	if err := floatValue.UnmarshalJSON([]byte("invalid")); err == nil {
		t.Fatal("invalid float JSON succeeded")
	}
	if !floatValue.Set || !floatValue.Valid || floatValue.Value != 9.5 {
		t.Fatalf("invalid float changed value: %+v", floatValue)
	}

	intValue := adminOptionalInt64{}
	if err := intValue.UnmarshalJSON([]byte("42")); err != nil {
		t.Fatal(err)
	}
	if !intValue.Set || !intValue.Valid || intValue.Value != 42 {
		t.Fatalf("int value = %+v", intValue)
	}
	if err := intValue.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatal(err)
	}
	if !intValue.Set || intValue.Valid || intValue.Value != 0 {
		t.Fatalf("null int value = %+v", intValue)
	}
	intValue = adminOptionalInt64{Valid: true, Value: 7}
	if err := intValue.UnmarshalJSON([]byte("1.5")); err == nil {
		t.Fatal("fractional int JSON succeeded")
	}
	if !intValue.Set || !intValue.Valid || intValue.Value != 7 {
		t.Fatalf("invalid int changed value: %+v", intValue)
	}
}
