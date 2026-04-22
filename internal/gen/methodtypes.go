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

// taskIDArgs is the `{list_id, taskseries_id, task_id}` trio
// every rtm.tasks.* write method uses to identify a target task.
// All three are integer IDs on the wire.
func taskIDArgs() map[string]argType {
	return map[string]argType{
		"list_id":       argTypeInt,
		"taskseries_id": argTypeInt,
		"task_id":       argTypeInt,
	}
}

// taskIDArgsPlus extends taskIDArgs() with method-specific
// typed arguments. Unknown keys clobber the ID entries if they
// overlap (they don't, in practice).
func taskIDArgsPlus(extras map[string]argType) map[string]argType {
	out := taskIDArgs()
	for k, v := range extras {
		out[k] = v
	}
	return out
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
	"rtm.contacts.delete":  {Arguments: map[string]argType{"contact_id": argTypeInt}},
	"rtm.contacts.getList": {Response: map[string]fieldType{"contacts.contact[].id": fieldTypeInt}},

	// ---- groups ----------------------------------------------
	"rtm.groups.add": {Response: map[string]fieldType{
		"group.id":                    fieldTypeInt,
		"group.contacts.contact[].id": fieldTypeInt,
	}},
	"rtm.groups.addContact": {Arguments: map[string]argType{
		"group_id":   argTypeInt,
		"contact_id": argTypeInt,
	}},
	"rtm.groups.delete": {Arguments: map[string]argType{"group_id": argTypeInt}},
	"rtm.groups.getList": {Response: map[string]fieldType{
		"groups.group[].id":                    fieldTypeInt,
		"groups.group[].contacts.contact[].id": fieldTypeInt,
	}},
	"rtm.groups.removeContact": {Arguments: map[string]argType{
		"group_id":   argTypeInt,
		"contact_id": argTypeInt,
	}},

	// ---- lists -----------------------------------------------
	"rtm.lists.add":     {Response: listResponseFields("list")},
	"rtm.lists.archive": {
		Arguments: map[string]argType{"list_id": argTypeInt},
		Response:  listResponseFields("list"),
	},
	"rtm.lists.delete": {
		Arguments: map[string]argType{"list_id": argTypeInt},
		Response:  listResponseFields("list"),
	},
	"rtm.lists.setDefaultList": {Arguments: map[string]argType{"list_id": argTypeInt}},
	"rtm.lists.setName": {
		Arguments: map[string]argType{"list_id": argTypeInt},
		Response:  listResponseFields("list"),
	},
	"rtm.lists.unarchive": {
		Arguments: map[string]argType{"list_id": argTypeInt},
		Response:  listResponseFields("list"),
	},
	"rtm.lists.getList": {Response: listResponseFields("lists.list[]")},

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
	"rtm.push.unsubscribe": {Arguments: map[string]argType{"subscription_id": argTypeInt}},

	// ---- reflection ------------------------------------------
	"rtm.reflection.getMethodInfo": {Response: map[string]fieldType{
		"method.needslogin":                    fieldTypeBool,
		"method.needssigning":                  fieldTypeBool,
		"method.requiredperms":                 fieldTypeInt,
		"method.arguments.argument[].optional": fieldTypeBool,
		"method.errors.error[].code":           fieldTypeInt,
	}},

	// ---- scripts ---------------------------------------------
	"rtm.scripts.add":    {Response: map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime}},
	"rtm.scripts.delete": {Arguments: map[string]argType{"script_id": argTypeInt}},
	"rtm.scripts.getList": {Response: map[string]fieldType{
		"scripts.script[].id":       fieldTypeInt,
		"scripts.script[].created":  fieldTypeTime,
		"scripts.script[].modified": fieldTypeTime,
	}},
	"rtm.scripts.run": {
		Arguments: map[string]argType{"script_id": argTypeInt},
		Response:  map[string]fieldType{"execution.id": fieldTypeInt},
	},
	"rtm.scripts.setCode": {
		Arguments: map[string]argType{"script_id": argTypeInt},
		Response:  map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime},
	},
	"rtm.scripts.setName": {
		Arguments: map[string]argType{"script_id": argTypeInt},
		Response:  map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime},
	},
	"rtm.scripts.setParams": {
		Arguments: map[string]argType{"script_id": argTypeInt},
		Response:  map[string]fieldType{"script.id": fieldTypeInt, "script.created": fieldTypeTime, "script.modified": fieldTypeTime},
	},

	// ---- settings --------------------------------------------
	"rtm.settings.getList": {Response: map[string]fieldType{
		"settings.dateformat":  fieldTypeInt,
		"settings.timeformat":  fieldTypeInt,
		"settings.defaultlist": fieldTypeInt,
		"settings.pro":         fieldTypeBool,
	}},

	// ---- tasks (write methods — single list wrapper) ---------
	// tasks.add accepts an optional list_id, name (string), parse
	// (bool) — plus parent_task_id / external_id. list_id is
	// technically optional on add; the generator still types it
	// when it's present.
	"rtm.tasks.add": {
		Arguments: map[string]argType{
			"list_id":        argTypeInt,
			"parse":          argTypeBool,
			"parent_task_id": argTypeInt,
		},
		Response: taskResponseFields("list"),
	},
	"rtm.tasks.addTags":      {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.complete":     {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.delete":       {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.movePriority": {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.moveTo": {
		Arguments: map[string]argType{
			"from_list_id":  argTypeInt,
			"to_list_id":    argTypeInt,
			"taskseries_id": argTypeInt,
			"task_id":       argTypeInt,
		},
		Response: taskResponseFields("list"),
	},
	"rtm.tasks.postpone":   {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.removeTags": {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.setDueDate": {
		Arguments: taskIDArgsPlus(map[string]argType{
			"has_due_time": argTypeBool,
			"parse":        argTypeBool,
		}),
		Response: taskResponseFields("list"),
	},
	"rtm.tasks.setEstimate": {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.setLocation": {
		Arguments: taskIDArgsPlus(map[string]argType{"location_id": argTypeInt}),
		Response:  taskResponseFields("list"),
	},
	"rtm.tasks.setName": {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.setParentTask": {
		Arguments: taskIDArgsPlus(map[string]argType{"parent_task_id": argTypeInt}),
		Response:  taskResponseFields("list"),
	},
	// setPriority's `priority` is enum ("1"/"2"/"3"/"N"); keep
	// it a string flag until the enum spec lands.
	"rtm.tasks.setPriority":   {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.setRecurrence": {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.setStartDate": {
		Arguments: taskIDArgsPlus(map[string]argType{
			"has_start_time": argTypeBool,
			"parse":          argTypeBool,
		}),
		Response: taskResponseFields("list"),
	},
	"rtm.tasks.setTags":    {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.setURL":     {Arguments: taskIDArgs(), Response: taskResponseFields("list")},
	"rtm.tasks.uncomplete": {Arguments: taskIDArgs(), Response: taskResponseFields("list")},

	// ---- tasks (read) ----------------------------------------
	"rtm.tasks.getList": {
		Arguments: map[string]argType{"list_id": argTypeInt},
		Response:  taskResponseFields("tasks.list[]"),
	},

	// ---- tasks.notes -----------------------------------------
	"rtm.tasks.notes.add": {
		Arguments: taskIDArgs(),
		Response:  map[string]fieldType{"note.id": fieldTypeInt, "note.created": fieldTypeTime, "note.modified": fieldTypeTime},
	},
	"rtm.tasks.notes.delete": {Arguments: map[string]argType{"note_id": argTypeInt}},
	"rtm.tasks.notes.edit": {
		Arguments: map[string]argType{"note_id": argTypeInt},
		Response:  map[string]fieldType{"note.id": fieldTypeInt, "note.created": fieldTypeTime, "note.modified": fieldTypeTime},
	},

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

	// ---- transactions ----------------------------------------
	"rtm.transactions.undo": {Arguments: map[string]argType{"transaction_id": argTypeInt}},

	// ---- synthetic fixture -----------------------------------
	"rtm.fixture.requiredOnly": {Response: map[string]fieldType{
		"item.id":     fieldTypeInt,
		"item.active": fieldTypeBool,
	}},
}
