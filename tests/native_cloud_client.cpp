#include <curl/curl.h>
#include <string>
#ifdef MSIME_WINDOWS_CLIENT
#include "cloud/cloud_request.h"
#else
#include "online/GoogleCloudProvider.h"
#include "online/HttpTransport.h"
#include <memory>
#endif
int main(int argc, char **argv) {
    if (argc != 3 || curl_global_init(CURL_GLOBAL_DEFAULT) != CURLE_OK) return 2;
    std::string result;
#ifdef MSIME_WINDOWS_CLIENT
    result = CloudIme::Fetch("ni'hao", false, {argv[1], argv[2]}, [] { return false; });
#else
    auto transport = std::make_shared<metasequoia::linux_ime::online::CurlHttpTransport>();
    metasequoia::linux_ime::online::GoogleCloudProvider provider(transport, {}, argv[1], argv[2]);
    metasequoia::OnlineQuery query;
    query.query_text = "ni'hao"; query.cloud_eligible = true;
    result = provider.fetch(query, [] { return false; }).value_or("");
#endif
    curl_global_cleanup();
    return result == "你好" ? 0 : 1;
}
