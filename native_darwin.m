#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#include <stdlib.h>
#include <string.h>
#include "_cgo_export.h"

static NSWindow *epWindow;
static NSView *epPackPage;
static NSView *epRestorePage;
static NSView *epPackDocument;
static NSView *epRestoreDocument;
static NSProgressIndicator *epProgress;
static NSMutableDictionary<NSNumber *, NSView *> *epControls;
static NSObject *epDelegate;
static BOOL epRunning;

static void epScrollToTop(NSScrollView *scroll, NSView *document) {
    NSClipView *clip = [scroll contentView];
    CGFloat y = MAX(0, NSHeight(document.bounds) - NSHeight(clip.bounds));
    [clip scrollToPoint:NSMakePoint(0, y)];
    [scroll reflectScrolledClipView:clip];
}

static NSString *epString(const char *text) {
    return text ? [NSString stringWithUTF8String:text] : @"";
}

static CGFloat epY(NSView *document, CGFloat top, CGFloat height) {
    return NSHeight(document.bounds) - top - height;
}

static void epRemember(int identifier, NSView *view) {
    if (!view) return;
    [view setTag:identifier];
    epControls[@(identifier)] = view;
}

@interface EasyPackageDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate>
@end

@implementation EasyPackageDelegate
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    (void)notification;
    [epWindow makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
    epRunning = YES;
}
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    (void)sender;
    return YES;
}
- (BOOL)windowShouldClose:(id)sender {
    (void)sender;
    easyPackageGoAction(11);
    return NO;
}
- (void)buttonAction:(id)sender {
    easyPackageGoAction((int)[sender tag]);
}
- (void)refresh:(id)sender {
    (void)sender;
    easyPackageGoRefresh();
}
- (void)quitMenu:(id)sender {
    (void)sender;
    easyPackageGoAction(11);
}
@end

static NSTextField *epLabel(NSView *parent, NSString *text, CGFloat x, CGFloat top, CGFloat width, CGFloat height, NSView *document) {
    NSTextField *label = [NSTextField labelWithString:text];
    [label setFrame:NSMakeRect(x, epY(document, top, height), width, height)];
    [label setTextColor:[NSColor colorWithRed:0.30 green:0.36 blue:0.46 alpha:1.0]];
    [parent addSubview:label];
    return label;
}

static void epSection(NSView *parent, NSString *text, CGFloat top, NSView *document) {
    NSTextField *label = [NSTextField labelWithString:text];
    [label setFrame:NSMakeRect(32, epY(document, top, 26), 820, 26)];
    [label setFont:[NSFont boldSystemFontOfSize:15]];
    [label setTextColor:[NSColor colorWithRed:0.14 green:0.22 blue:0.35 alpha:1.0]];
    [parent addSubview:label];
}

static NSTextField *epTextField(int identifier, BOOL secure, CGFloat x, CGFloat top,
                                CGFloat width, CGFloat height, NSView *parent, NSView *document) {
    NSTextField *field = secure
        ? (NSTextField *)[[NSSecureTextField alloc] initWithFrame:NSMakeRect(x, epY(document, top, height), width, height)]
        : [[NSTextField alloc] initWithFrame:NSMakeRect(x, epY(document, top, height), width, height)];
    [field setBezelStyle:NSTextFieldRoundedBezel];
    [field setFocusRingType:NSFocusRingTypeDefault];
    epRemember(identifier, field);
    [parent addSubview:field];
    return field;
}

static NSButton *epButton(int identifier, NSString *title, CGFloat x, CGFloat top,
                          CGFloat width, CGFloat height, NSView *parent, NSView *document) {
    NSButton *button = [NSButton buttonWithTitle:title target:epDelegate action:@selector(buttonAction:)];
    [button setFrame:NSMakeRect(x, epY(document, top, height), width, height)];
    [button setBezelStyle:NSBezelStyleRounded];
    epRemember(identifier, button);
    [parent addSubview:button];
    return button;
}

static NSButton *epCheck(int identifier, NSString *title, BOOL active, CGFloat x, CGFloat top,
                         CGFloat width, NSView *parent, NSView *document) {
    NSButton *button = [[NSButton alloc] initWithFrame:NSMakeRect(x, epY(document, top, 26), width, 26)];
    [button setButtonType:NSSwitchButton];
    [button setTitle:title];
    [button setState:active ? NSControlStateValueOn : NSControlStateValueOff];
    [button setTarget:epDelegate];
    [button setAction:@selector(buttonAction:)];
    epRemember(identifier, button);
    [parent addSubview:button];
    return button;
}

static NSPopUpButton *epCombo(int identifier, CGFloat x, CGFloat top, CGFloat width,
                              CGFloat height, NSView *parent, NSView *document) {
    NSPopUpButton *combo = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(x, epY(document, top, height), width, height) pullsDown:NO];
    [combo setTarget:epDelegate];
    [combo setAction:@selector(buttonAction:)];
    epRemember(identifier, combo);
    [parent addSubview:combo];
    return combo;
}

static void epBuildPackPage(void) {
    epPackDocument = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 916, 830)];
    epPackPage = epPackDocument;
    [epPackDocument setWantsLayer:YES];
    [[epPackDocument layer] setBackgroundColor:[[NSColor colorWithRed:0.96 green:0.97 blue:0.985 alpha:1] CGColor]];

    epSection(epPackDocument, @"01   选择要打包的项目", 26, epPackDocument);
    epLabel(epPackDocument, @"项目文件夹", 32, 67, 180, 26, epPackDocument);
    epTextField(100, NO, 220, 64, 555, 32, epPackDocument, epPackDocument);
    epButton(101, @"浏览…", 790, 64, 92, 32, epPackDocument, epPackDocument);
    epLabel(epPackDocument, @"常用 / 最近目录", 32, 110, 180, 26, epPackDocument);
    epCombo(102, 220, 107, 662, 32, epPackDocument, epPackDocument);

    epSection(epPackDocument, @"02   设置过滤规则", 158, epPackDocument);
    epCheck(103, @"自动过滤 .class 编译文件", YES, 32, 196, 290, epPackDocument, epPackDocument);
    epCheck(104, @"过滤超过此大小的文件", NO, 365, 196, 295, epPackDocument, epPackDocument);
    epLabel(epPackDocument, @"文件大小阈值", 32, 238, 180, 26, epPackDocument);
    NSTextField *limit = epTextField(105, NO, 220, 235, 92, 32, epPackDocument, epPackDocument);
    [limit setStringValue:@"100"];
    epCombo(106, 323, 235, 110, 32, epPackDocument, epPackDocument);
    NSTextField *exclude = [NSTextField labelWithString:@"默认排除 .git、.idea、.codegraph、.vscode、.vs、.svn、.hg、.fleet 及其内部文件"];
    [exclude setFrame:NSMakeRect(32, epY(epPackDocument, 278, 24), 850, 24)];
    [exclude setTextColor:[NSColor secondaryLabelColor]];
    [epPackDocument addSubview:exclude];

    epSection(epPackDocument, @"03   保存与安全", 326, epPackDocument);
    epLabel(epPackDocument, @"输出 ZIP / JPG 文件", 32, 367, 180, 26, epPackDocument);
    epTextField(107, NO, 220, 364, 555, 32, epPackDocument, epPackDocument);
    epButton(108, @"另存为…", 790, 364, 92, 32, epPackDocument, epPackDocument);
    epCheck(109, @"附加 JPG 封面", NO, 32, 414, 240, epPackDocument, epPackDocument);
    epLabel(epPackDocument, @"封面图片", 32, 454, 180, 26, epPackDocument);
    epTextField(110, NO, 220, 451, 555, 32, epPackDocument, epPackDocument);
    epButton(111, @"选择 JPG…", 790, 451, 92, 32, epPackDocument, epPackDocument);
    epCheck(112, @"启用 AES-256 压缩密码", NO, 32, 501, 300, epPackDocument, epPackDocument);
    epTextField(113, YES, 32, 540, 540, 32, epPackDocument, epPackDocument);
    epButton(114, @"显示", 584, 540, 80, 32, epPackDocument, epPackDocument);
    epButton(115, @"生成密码", 674, 540, 120, 32, epPackDocument, epPackDocument);
    epButton(116, @"预览文件", 32, 601, 130, 38, epPackDocument, epPackDocument);
    epButton(117, @"开始打包", 742, 601, 140, 38, epPackDocument, epPackDocument);
}

static void epBuildRestorePage(void) {
    epRestoreDocument = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 916, 830)];
    epRestorePage = epRestoreDocument;
    [epRestoreDocument setWantsLayer:YES];
    [[epRestoreDocument layer] setBackgroundColor:[[NSColor colorWithRed:0.96 green:0.97 blue:0.985 alpha:1] CGColor]];

    epSection(epRestoreDocument, @"01   选择要还原的 JPG 或 ZIP", 26, epRestoreDocument);
    epLabel(epRestoreDocument, @"支持普通 ZIP、AES ZIP 及 EasyPackage JPG。", 32, 65, 600, 26, epRestoreDocument);
    epLabel(epRestoreDocument, @"压缩文件", 32, 111, 180, 26, epRestoreDocument);
    epTextField(200, NO, 220, 108, 555, 32, epRestoreDocument, epRestoreDocument);
    epButton(201, @"选择文件…", 790, 108, 92, 32, epRestoreDocument, epRestoreDocument);

    epSection(epRestoreDocument, @"02   选择解压位置", 166, epRestoreDocument);
    epLabel(epRestoreDocument, @"解压到新文件夹，不会覆盖已有内容。", 32, 205, 600, 24, epRestoreDocument);
    epLabel(epRestoreDocument, @"目标父文件夹", 32, 248, 180, 26, epRestoreDocument);
    epTextField(202, NO, 220, 245, 555, 32, epRestoreDocument, epRestoreDocument);
    epButton(203, @"浏览…", 790, 245, 92, 32, epRestoreDocument, epRestoreDocument);
    epLabel(epRestoreDocument, @"常用 / 最近目录", 32, 291, 180, 26, epRestoreDocument);
    epCombo(210, 220, 288, 662, 32, epRestoreDocument, epRestoreDocument);
    epLabel(epRestoreDocument, @"新文件夹名称", 32, 336, 180, 26, epRestoreDocument);
    NSTextField *name = epTextField(204, NO, 220, 333, 662, 32, epRestoreDocument, epRestoreDocument);
    [name setStringValue:@"restored"];

    epSection(epRestoreDocument, @"03   解压密码（未加密文件留空）", 394, epRestoreDocument);
    epTextField(205, YES, 32, 434, 650, 32, epRestoreDocument, epRestoreDocument);
    epButton(206, @"显示", 694, 434, 92, 32, epRestoreDocument, epRestoreDocument);
    epButton(207, @"直接解压到新文件夹", 654, 501, 228, 40, epRestoreDocument, epRestoreDocument);
}

static void epBuildWindow(void) {
    NSRect frame = NSMakeRect(0, 0, 1200, 900);
    epWindow = [[NSWindow alloc] initWithContentRect:frame styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable) backing:NSBackingStoreBuffered defer:NO];
    [epWindow setTitle:@"EasyPackage · 轻松打包"];
    [epWindow setMinSize:NSMakeSize(1160, 680)];
    [epWindow setBackgroundColor:[NSColor colorWithRed:0.96 green:0.97 blue:0.985 alpha:1]];
    [epWindow center];
    [epWindow setDelegate:(id<NSWindowDelegate>)epDelegate];

    NSView *root = [epWindow contentView];
    CGFloat width = NSWidth(root.bounds), height = NSHeight(root.bounds);
    NSView *header = [[NSView alloc] initWithFrame:NSMakeRect(0, height - 104, width, 104)];
    [header setAutoresizingMask:NSViewWidthSizable | NSViewMinYMargin];
    [header setWantsLayer:YES];
    [[header layer] setBackgroundColor:[[NSColor whiteColor] CGColor]];
    NSTextField *title = [NSTextField labelWithString:@"EasyPackage"];
    [title setFrame:NSMakeRect(28, 51, 400, 34)];
    [title setFont:[NSFont boldSystemFontOfSize:23]];
    [title setTextColor:[NSColor colorWithRed:0.12 green:0.19 blue:0.30 alpha:1]];
    [header addSubview:title];
    NSTextField *subtitle = [NSTextField labelWithString:@"本机项目打包与还原 · 文件仅在本机处理"];
    [subtitle setFrame:NSMakeRect(29, 27, 460, 20)];
    [subtitle setTextColor:[NSColor secondaryLabelColor]];
    [header addSubview:subtitle];
    [root addSubview:header];

    NSView *sidebar = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 218, height - 104)];
    [sidebar setAutoresizingMask:NSViewHeightSizable | NSViewMaxXMargin];
    [sidebar setWantsLayer:YES];
    [[sidebar layer] setBackgroundColor:[[NSColor colorWithRed:0.925 green:0.945 blue:0.975 alpha:1] CGColor]];
    epButton(12, @"◈   项目打包", 18, 26, 182, 42, sidebar, sidebar);
    epButton(13, @"↻   JPG / ZIP 还原", 18, 80, 182, 42, sidebar, sidebar);
    epButton(11, @"退出", 18, height - 160, 182, 38, sidebar, sidebar);
    [root addSubview:sidebar];

    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSMakeRect(218, 62, width - 218, height - 166)];
    [scroll setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
    [scroll setHasVerticalScroller:YES];
    [scroll setHasHorizontalScroller:NO];
    [scroll setBorderType:NSNoBorder];
    epBuildPackPage();
    epBuildRestorePage();
    [scroll setDocumentView:epPackDocument];
    [root addSubview:scroll];

    NSView *footer = [[NSView alloc] initWithFrame:NSMakeRect(218, 0, width - 218, 62)];
    [footer setAutoresizingMask:NSViewWidthSizable | NSViewMaxYMargin];
    [footer setWantsLayer:YES];
    [[footer layer] setBackgroundColor:[[NSColor whiteColor] CGColor]];
    NSTextField *status = [NSTextField labelWithString:@"准备就绪 · 选择项目文件夹开始"];
    [status setFrame:NSMakeRect(24, 20, 350, 22)];
    [status setLineBreakMode:NSLineBreakByTruncatingMiddle];
    epRemember(300, status);
    [footer addSubview:status];
    CGFloat footerWidth = width - 218;
    epProgress = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(footerWidth - 395, 22, 155, 12)];
    [epProgress setAutoresizingMask:NSViewMinXMargin];
    [epProgress setIndeterminate:YES];
    [epProgress setStyle:NSProgressIndicatorStyleBar];
    [epProgress startAnimation:nil];
    epRemember(301, epProgress);
    [footer addSubview:epProgress];
    [epButton(208, @"取消", footerWidth - 222, 14, 76, 34, footer, footer) setAutoresizingMask:NSViewMinXMargin];
    [epButton(209, @"查看位置", footerWidth - 132, 14, 108, 34, footer, footer) setAutoresizingMask:NSViewMinXMargin];
    [root addSubview:footer];
    [epControls[@208] setHidden:YES];
    [epControls[@209] setEnabled:NO];
}

static void epSetupMenu(void) {
    NSMenu *mainMenu = [[NSMenu alloc] initWithTitle:@"EasyPackage"];
    NSMenuItem *appMenuItem = [[NSMenuItem alloc] initWithTitle:@"EasyPackage" action:nil keyEquivalent:@""];
    NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"EasyPackage"];
    NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"退出 EasyPackage" action:@selector(quitMenu:) keyEquivalent:@"q"];
    [quit setTarget:epDelegate];
    [appMenu addItem:quit];
    [appMenuItem setSubmenu:appMenu];
    [mainMenu addItem:appMenuItem];
    [NSApp setMainMenu:mainMenu];
}

void ep_run(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        epControls = [[NSMutableDictionary alloc] init];
        epDelegate = [[EasyPackageDelegate alloc] init];
        [NSApp setDelegate:(id<NSApplicationDelegate>)epDelegate];
        epSetupMenu();
        epBuildWindow();
        [epPackDocument setFrameSize:NSMakeSize(916, 830)];
        [epRestoreDocument setFrameSize:NSMakeSize(916, 830)];
        epPackPage = epPackDocument;
        epRestorePage = epRestoreDocument;
        [epRestorePage setHidden:YES];
        for (NSView *view in [[epWindow contentView] subviews]) {
            if ([view isKindOfClass:[NSScrollView class]]) {
                epScrollToTop((NSScrollView *)view, epPackDocument);
            }
        }
        easyPackageGoReady();
        [NSTimer scheduledTimerWithTimeInterval:0.18 target:epDelegate selector:@selector(refresh:) userInfo:nil repeats:YES];
        [NSApp run];
        epRunning = NO;
    }
}

void ep_set_text(int identifier, const char *text) {
    NSView *view = epControls[@(identifier)];
    NSString *value = epString(text);
    if ([view isKindOfClass:[NSTextField class]]) [(NSTextField *)view setStringValue:value];
    else if ([view isKindOfClass:[NSButton class]]) [(NSButton *)view setTitle:value];
}

const char *ep_get_text(int identifier) {
    NSView *view = epControls[@(identifier)];
    NSString *value = nil;
    if ([view isKindOfClass:[NSTextField class]]) value = [(NSTextField *)view stringValue];
    if (!value) return "";
    return [value UTF8String] ?: "";
}

int ep_checked(int identifier) {
    NSView *view = epControls[@(identifier)];
    if ([view isKindOfClass:[NSButton class]]) return [(NSButton *)view state] == NSControlStateValueOn;
    return 0;
}

void ep_set_secret(int identifier, int secret) {
    NSView *old = epControls[@(identifier)];
    if (![old isKindOfClass:[NSTextField class]]) return;
    BOOL currentlySecure = [old isKindOfClass:[NSSecureTextField class]];
    if (currentlySecure == (secret != 0)) return;
    NSString *value = [(NSTextField *)old stringValue];
    NSView *parent = [old superview];
    NSRect frame = [old frame];
    NSTextField *replacement = secret
        ? (NSTextField *)[[NSSecureTextField alloc] initWithFrame:frame]
        : [[NSTextField alloc] initWithFrame:frame];
    [replacement setBezelStyle:NSTextFieldRoundedBezel];
    [replacement setStringValue:value];
    [replacement setTag:identifier];
    [parent replaceSubview:old with:replacement];
    epControls[@(identifier)] = replacement;
}

void ep_set_enabled(int identifier, int enabled) {
    NSView *view = epControls[@(identifier)];
    if ([view respondsToSelector:@selector(setEnabled:)]) [(id)view setEnabled:enabled != 0];
}

void ep_set_status(const char *text, int percent, int marquee) {
    NSView *view = epControls[@300];
    if ([view isKindOfClass:[NSTextField class]]) [(NSTextField *)view setStringValue:epString(text)];
    if (marquee) {
        [epProgress setIndeterminate:YES];
        [epProgress startAnimation:nil];
    } else {
        [epProgress stopAnimation:nil];
        [epProgress setIndeterminate:NO];
        [epProgress setDoubleValue:percent];
    }
}

void ep_set_busy(int busy) {
    NSView *cancel = epControls[@208];
    if (cancel) [cancel setHidden:busy == 0];
}

void ep_set_page(int page) {
    BOOL restore = page == 1;
    [epPackPage setHidden:restore];
    [epRestorePage setHidden:!restore];
    NSScrollView *scroll = nil;
    for (NSView *view in [[epWindow contentView] subviews]) {
        if ([view isKindOfClass:[NSScrollView class]]) scroll = (NSScrollView *)view;
    }
    [scroll setDocumentView:restore ? epRestoreDocument : epPackDocument];
    epScrollToTop(scroll, restore ? epRestoreDocument : epPackDocument);
}

void ep_set_options(int identifier, char **items, int count, int active) {
    NSView *view = epControls[@(identifier)];
    if (![view isKindOfClass:[NSPopUpButton class]]) return;
    NSPopUpButton *combo = (NSPopUpButton *)view;
    [combo removeAllItems];
    for (int i = 0; i < count; ++i) [combo addItemWithTitle:epString(items[i])];
    if (active >= 0 && active < count) [combo selectItemAtIndex:active];
    else [combo selectItem:nil];
}

int ep_selected(int identifier) {
    NSView *view = epControls[@(identifier)];
    if (![view isKindOfClass:[NSPopUpButton class]]) return -1;
    return (int)[(NSPopUpButton *)view indexOfSelectedItem];
}

char *ep_get_selected_text(int identifier) {
    NSView *view = epControls[@(identifier)];
    if (![view isKindOfClass:[NSPopUpButton class]]) return NULL;
    NSString *value = [(NSPopUpButton *)view titleOfSelectedItem];
    return value ? strdup([value UTF8String]) : NULL;
}

int ep_choose(int kind, const char *current, char **selected) {
    NSString *path = epString(current);
    if (kind == 3 || kind == 5) {
        NSSavePanel *panel = [NSSavePanel savePanel];
        [panel setTitle:@"保存 EasyPackage 压缩包"];
        [panel setCanCreateDirectories:YES];
        [panel setAllowedFileTypes:kind == 5 ? @[@"jpg", @"jpeg"] : @[@"zip"]];
        [panel setNameFieldStringValue:path.length ? path.lastPathComponent : (kind == 5 ? @"package.jpg" : @"package.zip")];
        if (path.length) [panel setDirectoryURL:[NSURL fileURLWithPath:path.stringByDeletingLastPathComponent isDirectory:YES]];
        if ([panel runModal] != NSModalResponseOK) return 0;
        *selected = strdup([panel.URL.path UTF8String]);
        return 1;
    }
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    [panel setCanChooseDirectories:kind == 1];
    [panel setCanChooseFiles:kind != 1];
    [panel setAllowsMultipleSelection:NO];
    [panel setResolvesAliases:NO];
    if (kind == 2) [panel setAllowedFileTypes:@[@"jpg", @"jpeg"]];
    if (kind == 4) [panel setAllowedFileTypes:@[@"jpg", @"jpeg", @"zip"]];
    if (path.length) {
        NSString *directory = kind == 1 ? path : path.stringByDeletingLastPathComponent;
        [panel setDirectoryURL:[NSURL fileURLWithPath:directory isDirectory:YES]];
    }
    if ([panel runModal] != NSModalResponseOK) return 0;
    *selected = strdup([panel.URL.path UTF8String]);
    return 1;
}

int ep_message(const char *title, const char *message, int confirm) {
    NSAlert *alert = [[NSAlert alloc] init];
    [alert setMessageText:epString(title)];
    [alert setInformativeText:epString(message)];
    [alert addButtonWithTitle:confirm ? @"是" : @"好"];
    if (confirm) [alert addButtonWithTitle:@"否"];
    return [alert runModal] == NSAlertFirstButtonReturn;
}

void ep_quit(void) {
    if (!epRunning) return;
    [epWindow orderOut:nil];
    [NSApp stop:nil];
    NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0 context:nil subtype:0 data1:0 data2:0];
    [NSApp postEvent:event atStart:YES];
}
