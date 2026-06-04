#include <Carbon/Carbon.h>

extern void lylatlinkHotkeyPressed(void);

static EventHotKeyRef hotKeyRef;
static EventHandlerRef handlerRef;

static OSStatus hotKeyHandler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
	(void)nextHandler;
	(void)userData;

	EventHotKeyID hotKeyID;
	OSStatus status = GetEventParameter(
		event,
		kEventParamDirectObject,
		typeEventHotKeyID,
		NULL,
		sizeof(hotKeyID),
		NULL,
		&hotKeyID
	);
	if (status != noErr) {
		return status;
	}

	if (hotKeyID.signature == 'LylL' && hotKeyID.id == 1) {
		lylatlinkHotkeyPressed();
		return noErr;
	}
	return eventNotHandledErr;
}

OSStatus lylatlink_register_hotkey(UInt32 keyCode, UInt32 modifiers) {
	EventTypeSpec eventType = { kEventClassKeyboard, kEventHotKeyPressed };
	OSStatus status = InstallApplicationEventHandler(
		&hotKeyHandler,
		1,
		&eventType,
		NULL,
		&handlerRef
	);
	if (status != noErr) {
		return status;
	}

	EventHotKeyID hotKeyID = { 'LylL', 1 };
	status = RegisterEventHotKey(
		keyCode,
		modifiers,
		hotKeyID,
		GetApplicationEventTarget(),
		0,
		&hotKeyRef
	);
	if (status != noErr) {
		if (handlerRef != NULL) {
			RemoveEventHandler(handlerRef);
			handlerRef = NULL;
		}
	}
	return status;
}

void lylatlink_unregister_hotkey(void) {
	if (hotKeyRef != NULL) {
		UnregisterEventHotKey(hotKeyRef);
		hotKeyRef = NULL;
	}
	if (handlerRef != NULL) {
		RemoveEventHandler(handlerRef);
		handlerRef = NULL;
	}
}
