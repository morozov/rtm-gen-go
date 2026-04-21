package gen

// fieldType is the semantic type a response JSON field is given
// in the generated Go response struct. See spec 008.
type fieldType int

const (
	fieldTypeString fieldType = iota // the default — `string` field
	fieldTypeBool                    // → `rtmBool`
	fieldTypeInt                     // → `rtmInt`
	fieldTypeTime                    // → `*rtmTime` (pointer for absent-vs-present)
)

// argType is the semantic type a user-visible cobra flag will be
// declared with. Reserved for spec 009; unused by spec 008.
type argType int

const (
	argTypeString argType = iota
	argTypeBool
	argTypeInt
)

// methodTypeInfo groups a single RTM method's wire-type info.
// Arguments feeds spec 009 (typed cobra flags); Response feeds
// spec 008 (typed response structs). The two halves are
// independently maintained; their co-location is a readability
// choice.
type methodTypeInfo struct {
	Arguments map[string]argType
	Response  map[string]fieldType
}

// listResponseFields describes the attribute set every `<list>`
// element from rtm.lists.* returns. The same attributes appear
// whether the method returns a single list (`list.*`) or an
// array under a collection wrapper (`lists.list[].*`).
func listResponseFields(prefix string) map[string]fieldType {
	return map[string]fieldType{
		prefix + ".id":         fieldTypeInt,
		prefix + ".deleted":    fieldTypeBool,
		prefix + ".locked":     fieldTypeBool,
		prefix + ".archived":   fieldTypeBool,
		prefix + ".position":   fieldTypeInt,
		prefix + ".smart":      fieldTypeBool,
		prefix + ".sort_order": fieldTypeInt,
	}
}

// taskResponseFields describes the shape every task-returning
// rtm.tasks.* method produces. Write methods return `list.*`
// (one list wrapping one taskseries); `rtm.tasks.getList` returns
// the fuller `tasks.list[].*`.
//
// Timestamp fields (`added`, `completed`, `created`, `modified`,
// `due`, `start`, `deleted`) are typed as `fieldTypeTime` —
// `rtmTime` handles RTM's "" → null convention. `priority` stays
// stringly-typed (enum "1"/"2"/"3"/"N"). `estimate` stays a
// string (free-form duration like `"5 minutes"`).
func taskResponseFields(prefix string) map[string]fieldType {
	return map[string]fieldType{
		prefix + ".id":                                  fieldTypeInt,
		prefix + ".taskseries[].id":                     fieldTypeInt,
		prefix + ".taskseries[].created":                fieldTypeTime,
		prefix + ".taskseries[].modified":               fieldTypeTime,
		prefix + ".taskseries[].task[].id":              fieldTypeInt,
		prefix + ".taskseries[].task[].has_due_time":    fieldTypeBool,
		prefix + ".taskseries[].task[].has_start_time":  fieldTypeBool,
		prefix + ".taskseries[].task[].postponed":       fieldTypeInt,
		prefix + ".taskseries[].task[].added":           fieldTypeTime,
		prefix + ".taskseries[].task[].completed":       fieldTypeTime,
		prefix + ".taskseries[].task[].deleted":         fieldTypeTime,
		prefix + ".taskseries[].task[].due":             fieldTypeTime,
		prefix + ".taskseries[].task[].start":           fieldTypeTime,
	}
}

// typeTable is the committed per-method type table. Structure
// (field names, hierarchy, array-ness) is inferred from the
// reflection sample responses; this table overrides types.
// Fields absent from a method's entry — or methods absent from
// the table entirely — default to `fieldTypeString`.
var typeTable = map[string]methodTypeInfo{
	// ---- auth ------------------------------------------------
	"rtm.auth.checkToken": {Response: map[string]fieldType{"auth.user.id": fieldTypeInt}},
	"rtm.auth.getToken":   {Response: map[string]fieldType{"auth.user.id": fieldTypeInt}},
	// auth.getFrob — frob is a hex-string nonce, stays string.

	// ---- contacts --------------------------------------------
	"rtm.contacts.add":     {Response: map[string]fieldType{"contact.id": fieldTypeInt}},
	"rtm.contacts.getList": {Response: map[string]fieldType{"contacts.contact[].id": fieldTypeInt}},

	// ---- groups ----------------------------------------------
	"rtm.groups.add": {Response: map[string]fieldType{
		"group.id":                    fieldTypeInt,
		"group.contacts.contact[].id": fieldTypeInt,
	}},
	"rtm.groups.getList": {Response: map[string]fieldType{
		"groups.group[].id":                    fieldTypeInt,
		"groups.group[].contacts.contact[].id": fieldTypeInt,
	}},

	// ---- lists -----------------------------------------------
	"rtm.lists.add":       {Response: listResponseFields("list")},
	"rtm.lists.archive":   {Response: listResponseFields("list")},
	"rtm.lists.delete":    {Response: listResponseFields("list")},
	"rtm.lists.setName":   {Response: listResponseFields("list")},
	"rtm.lists.unarchive": {Response: listResponseFields("list")},
	"rtm.lists.getList":   {Response: listResponseFields("lists.list[]")},

	// ---- locations -------------------------------------------
	"rtm.locations.getList": {Response: map[string]fieldType{
		"locations.location[].id":       fieldTypeInt,
		"locations.location[].viewable": fieldTypeBool,
		"locations.location[].zoom":     fieldTypeInt,
	}},

	// ---- push ------------------------------------------------
	"rtm.push.getSubscriptions": {Response: map[string]fieldType{
		"subscriptions.subscription[].id":      fieldTypeInt,
		"subscriptions.subscription[].pending": fieldTypeBool,
	}},
	"rtm.push.subscribe": {Response: map[string]fieldType{
		"subscription.id":      fieldTypeInt,
		"subscription.pending": fieldTypeBool,
	}},

	// ---- reflection ------------------------------------------
	"rtm.reflection.getMethodInfo": {Response: map[string]fieldType{
		"method.needslogin":                    fieldTypeBool,
		"method.needssigning":                  fieldTypeBool,
		"method.requiredperms":                 fieldTypeInt,
		"method.arguments.argument[].optional": fieldTypeBool,
		"method.errors.error[].code":           fieldTypeInt,
	}},

	// ---- scripts ---------------------------------------------
	"rtm.scripts.add":       {Response: map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime}},
	"rtm.scripts.getList":   {Response: map[string]fieldType{"scripts.script[].id": fieldTypeInt, "scripts.script[].created": fieldTypeTime, "scripts.script[].modified": fieldTypeTime}},
	"rtm.scripts.run":       {Response: map[string]fieldType{"execution.id": fieldTypeInt}},
	"rtm.scripts.setCode":   {Response: map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime}},
	"rtm.scripts.setName":   {Response: map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime}},
	"rtm.scripts.setParams": {Response: map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime}},

	// ---- settings --------------------------------------------
	"rtm.settings.getList": {Response: map[string]fieldType{
		"settings.dateformat":  fieldTypeInt,
		"settings.timeformat":  fieldTypeInt,
		"settings.defaultlist": fieldTypeInt,
		"settings.pro":         fieldTypeBool,
	}},

	// ---- tasks (write methods — single list wrapper) ---------
	"rtm.tasks.add":           {Response: taskResponseFields("list")},
	"rtm.tasks.addTags":       {Response: taskResponseFields("list")},
	"rtm.tasks.complete":      {Response: taskResponseFields("list")},
	"rtm.tasks.delete":        {Response: taskResponseFields("list")},
	"rtm.tasks.movePriority":  {Response: taskResponseFields("list")},
	"rtm.tasks.moveTo":        {Response: taskResponseFields("list")},
	"rtm.tasks.postpone":      {Response: taskResponseFields("list")},
	"rtm.tasks.removeTags":    {Response: taskResponseFields("list")},
	"rtm.tasks.setDueDate":    {Response: taskResponseFields("list")},
	"rtm.tasks.setEstimate":   {Response: taskResponseFields("list")},
	"rtm.tasks.setLocation":   {Response: taskResponseFields("list")},
	"rtm.tasks.setName":       {Response: taskResponseFields("list")},
	"rtm.tasks.setParentTask": {Response: taskResponseFields("list")},
	"rtm.tasks.setPriority":   {Response: taskResponseFields("list")},
	"rtm.tasks.setRecurrence": {Response: taskResponseFields("list")},
	"rtm.tasks.setStartDate":  {Response: taskResponseFields("list")},
	"rtm.tasks.setTags":       {Response: taskResponseFields("list")},
	"rtm.tasks.setURL":        {Response: taskResponseFields("list")},
	"rtm.tasks.uncomplete":    {Response: taskResponseFields("list")},

	// ---- tasks (read) ----------------------------------------
	"rtm.tasks.getList": {Response: taskResponseFields("tasks.list[]")},

	// ---- tasks.notes -----------------------------------------
	"rtm.tasks.notes.add":  {Response: map[string]fieldType{"note.id": fieldTypeInt, "note.created": fieldTypeTime, "note.modified": fieldTypeTime}},
	"rtm.tasks.notes.edit": {Response: map[string]fieldType{"note.id": fieldTypeInt, "note.created": fieldTypeTime, "note.modified": fieldTypeTime}},

	// ---- test ------------------------------------------------
	"rtm.test.login": {Response: map[string]fieldType{"user.id": fieldTypeInt}},

	// ---- timelines -------------------------------------------
	"rtm.timelines.create": {Response: map[string]fieldType{"timeline": fieldTypeInt}},

	// ---- timezones -------------------------------------------
	"rtm.timezones.getList": {Response: map[string]fieldType{
		"timezones.timezone[].id":             fieldTypeInt,
		"timezones.timezone[].dst":            fieldTypeBool,
		"timezones.timezone[].offset":         fieldTypeInt,
		"timezones.timezone[].current_offset": fieldTypeInt,
	}},

	// ---- synthetic fixture -----------------------------------
	"rtm.fixture.requiredOnly": {Response: map[string]fieldType{
		"item.id":     fieldTypeInt,
		"item.active": fieldTypeBool,
	}},
}
