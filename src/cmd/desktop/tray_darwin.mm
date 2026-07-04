#import <Cocoa/Cocoa.h>

extern void goTrayShow(void);
extern void goTrayHide(void);
extern void goTrayQuit(void);

@interface KVNStatusItemController : NSObject {
    NSStatusItem *statusItem;
    BOOL isConnected;
}
- (instancetype)initWithImageData:(NSData *)data;
- (void)setConnected:(BOOL)connected;
- (void)cleanup;
@end

@implementation KVNStatusItemController

- (instancetype)initWithImageData:(NSData *)data {
    self = [super init];
    if (self) {
        isConnected = YES;
        statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];

        NSImage *icon = [[NSImage alloc] initWithData:data];
        [icon setTemplate:YES];
        statusItem.button.image = icon;
        [icon release];

        NSMenu *menu = [[NSMenu alloc] init];

        NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"Show"
                                                          action:@selector(onShow:)
                                                   keyEquivalent:@""];
        showItem.target = self;
        [menu addItem:showItem];
        [showItem release];

        NSMenuItem *hideItem = [[NSMenuItem alloc] initWithTitle:@"Hide"
                                                          action:@selector(onHide:)
                                                   keyEquivalent:@""];
        hideItem.target = self;
        [menu addItem:hideItem];
        [hideItem release];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"Quit"
                                                          action:@selector(onQuit:)
                                                   keyEquivalent:@"q"];
        quitItem.target = self;
        [menu addItem:quitItem];
        [quitItem release];

        statusItem.menu = menu;
        [menu release];
    }
    return self;
}

- (void)onShow:(id)sender {
    goTrayShow();
}

- (void)onHide:(id)sender {
    goTrayHide();
}

- (void)onQuit:(id)sender {
    goTrayQuit();
}

- (void)setConnected:(BOOL)connected {
    isConnected = connected;
}

- (void)cleanup {
    [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
    statusItem = nil;
}

- (void)dealloc {
    [self cleanup];
    [super dealloc];
}

@end

void* createTray(const char *pngData, int pngLen) {
    NSData *data = [NSData dataWithBytes:pngData length:pngLen];
    KVNStatusItemController *ctrl = [[KVNStatusItemController alloc] initWithImageData:data];
    return (__bridge_retained void*)ctrl;
}

void setTrayStatus(void *tray, int connected) {
    if (!tray) return;
    KVNStatusItemController *ctrl = (__bridge KVNStatusItemController*)tray;
    [ctrl setConnected:(BOOL)connected];
}

void destroyTray(void *tray) {
    if (!tray) return;
    KVNStatusItemController *ctrl = (__bridge_transfer KVNStatusItemController*)tray;
    [ctrl cleanup];
    [ctrl release];
}
