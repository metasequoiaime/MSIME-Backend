#import <Foundation/Foundation.h>
#import <Security/Security.h>
#import "MSIMEBackendClient.h"

// 仅用于测试进程的配置和信任锚，不写入系统信任或钥匙串。
static NSString *baseURL, *deviceToken;
static NSData *certificateDER;
static BOOL pinTestCertificate = YES;
static void Require(BOOL condition, NSString *message) {
    if (!condition) { fprintf(stderr, "%s\n", message.UTF8String); exit(1); }
}
@interface NetworkBackend : MSIMEBackendClient @end
@implementation NetworkBackend
- (void)reloadConfiguration {
    [self cancel];
    [self setValue:baseURL forKey:@"baseURL"];
    [self setValue:deviceToken forKey:@"token"];
    [self setValue:@YES forKey:@"enabled"];
}
- (void)URLSession:(NSURLSession *)session didReceiveChallenge:(NSURLAuthenticationChallenge *)challenge
 completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition, NSURLCredential *))completion {
    (void)session;
    if (!pinTestCertificate || ![challenge.protectionSpace.authenticationMethod isEqual:NSURLAuthenticationMethodServerTrust]) {
        completion(NSURLSessionAuthChallengePerformDefaultHandling, nil); return;
    }
    SecTrustRef trust = challenge.protectionSpace.serverTrust;
    SecCertificateRef certificate = SecCertificateCreateWithData(NULL, (__bridge CFDataRef)certificateDER);
    Require(certificate != NULL && trust != NULL, @"missing test trust material");
    NSArray *anchors = @[(__bridge id)certificate];
    OSStatus status = SecTrustSetAnchorCertificates(trust, (__bridge CFArrayRef)anchors);
    CFRelease(certificate);
    Require(status == errSecSuccess && SecTrustSetAnchorCertificatesOnly(trust, true) == errSecSuccess, @"cannot set process-local trust anchor");
    if (SecTrustEvaluateWithError(trust, NULL)) completion(NSURLSessionAuthChallengeUseCredential, [NSURLCredential credentialForTrust:trust]);
    else completion(NSURLSessionAuthChallengeCancelAuthenticationChallenge, nil);
}
@end
static void Pump(NSTimeInterval seconds) {
    NSDate *end = [NSDate dateWithTimeIntervalSinceNow:seconds];
    while (end.timeIntervalSinceNow > 0) [NSRunLoop.currentRunLoop runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
}
static void Request(NetworkBackend *client, NSString *text, BOOL japanese, NSString *expected) {
    __block BOOL completed = NO;
    [client cloudCandidateForText:text japanese:japanese completion:^(NSString *candidate) {
        Require(NSThread.isMainThread, @"completion left main thread");
        Require((expected == nil && candidate == nil) || [candidate isEqual:expected], @"candidate response mismatch");
        completed = YES;
    }];
    NSDate *end = [NSDate dateWithTimeIntervalSinceNow:6];
    while (!completed && end.timeIntervalSinceNow > 0) Pump(0.01);
    Require(completed, @"network callback timed out");
}
int main(int argc, char **argv) { @autoreleasepool {
    if (argc != 4) return 2;
    baseURL = @(argv[1]); deviceToken = @(argv[2]);
    certificateDER = [NSData dataWithContentsOfFile:@(argv[3])];
    Require(certificateDER != nil, @"test certificate unreadable");
    NetworkBackend *client = [NetworkBackend new];
    Request(client, @"ni'hao", NO, @"你好");
    Request(client, @"nihon", YES, @"日本");
    puts("Apple NSURLSession TLS candidates and main-thread delivery passed");
    deviceToken = @"invalid-synthetic-device-token"; [client reloadConfiguration];
    Request(client, @"ni'hao", NO, nil);
    puts("Apple rejected credential passed");
    deviceToken = @(argv[2]); [client reloadConfiguration];
    [client cloudCandidateForText:@"cancel" japanese:NO completion:^(NSString *candidate) {
        (void)candidate; Require(NO, @"cancelled request delivered a candidate");
    }];
    Pump(0.9); [client cancel]; Pump(0.3);
    Request(client, @"ni'hao", NO, @"你好");
    puts("Apple in-flight cancellation and next generation passed");
    pinTestCertificate = NO;
    Request(client, @"ni'hao", NO, nil);
    puts("Apple untrusted TLS rejected passed");
    [client cancel];
} return 0; }
