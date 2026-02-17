#include <Inkplate.h>

/*
   Inkplate10_Show_JPG_With_HTTPClient example for Soldered Inkplate 10
   For this example you will need a USB-C cable, Inkplate 10, and an available WiFi connection.
   Select "e-radionica Inkplate10" or "Soldered Inkplate10" from Tools -> Board menu.
   Don't have "e-radionica Inkplate10" or "Soldered Inkplate10" option? Follow our tutorial and add it:
   https://soldered.com/learn/add-inkplate-6-board-definition-to-arduino-ide/

   This example will show you how to show JPG image using HTTPClient.
   Make sure that you entered WiFi credentials and change the image link if you want any other image.

   Want to learn more about Inkplate? Visit www.inkplate.io
   Looking to get support? Write on our forums: https://forum.soldered.com/
   17 April 2023 by Soldered
*/

// Next 3 lines are a precaution, you can ignore those, and the example would also work without them
#if !defined(ARDUINO_INKPLATE10) && !defined(ARDUINO_INKPLATE10V2)
#error                                                                                                                 \
    "Wrong board selection for this example, please select e-radionica Inkplate10 or Soldered Inkplate10 in the boards menu."
#endif

// NOTE: This sketch avoids the Arduino String class to prevent heap fragmentation on ESP32.
// All strings use const char* (for literals) or fixed-size char[] buffers instead.
// See: https://cpp4arduino.com/2018/11/06/what-is-heap-fragmentation.html
//      https://cpp4arduino.com/2018/11/21/eight-tips-to-use-the-string-class-efficiently.html

// Include needed libraries in the sketch
#include "HTTPClient.h"
#include "Inkplate.h"
#include "WiFi.h"
#include "esp_task_wdt.h"

// Create an object on Inkplate library and also set library into 1 Bit mode (BW)
Inkplate display(INKPLATE_1BIT);

/**************** CHANGE HERE ******************/
#include "config_secret.h"  // Copy config_secret.h.example to config_secret.h and add ssid, pass, url

// Here you can change the interval of updating the image.
#define UPDATE_INTERVAL_IN_SECS 300

// Retry settings for image loading with exponential backoff (for HTTP/network failures)
#define MAX_RETRIES 5
#define RETRY_MIN_DELAY_MS 2000UL   // Start with 2 seconds
#define RETRY_MAX_DELAY_MS 60000UL  // Cap at 60 seconds
#define RETRY_BACKOFF_MULTIPLIER 2UL // Double the delay each time

// Watchdog timeout in seconds. 120s is enough: we reset after each http.GET() (max 60s)
// and every 2s during backoff, so we never go >60s between resets in the retry path.
#define WDT_TIMEOUT_SECS 120

// Reset WDT at least this often during long delays (backoff) so we never exceed WDT timeout
#define WDT_RESET_INTERVAL_MS 2000

// Reject responses larger than this (getSize() can be wrong or malicious); dashboard image is typically much smaller
#define MAX_IMAGE_SIZE (2 * 1024 * 1024)

// Max bytes of HTTP error body to read and show (avoid OOM on error responses)
#define MAX_ERROR_BODY_LEN 512

// Error overlay height (px); needs to fit several lines of text
#define ERROR_OVERLAY_HEIGHT 90

/***********************************************/

// Variable that holds last connection time
unsigned long lastConnectionTime = 0;

// Track if we've successfully loaded an image at least once
bool hasLoadedImage = false;


void setup()
{
    display.begin();             // Init Inkplate library (you should call this function ONLY ONCE)
    display.clearDisplay();      // Clear frame buffer of display
    display.setTextSize(2);      // Set text size to be 2 times bigger than original (5x7 px)
    display.setTextColor(BLACK); // Set text color to black

    Serial.begin(115200);

    // Initialize watchdog timer
    esp_task_wdt_init(WDT_TIMEOUT_SECS, true); // Enable panic so ESP32 restarts
    esp_task_wdt_add(NULL);                    // Add current thread to WDT watch
    Serial.print("Watchdog initialized with ");
    Serial.print(WDT_TIMEOUT_SECS);
    Serial.println("s timeout");

    // Let's connect to the WiFi
    // Show a connection message
    display.print("Connecting to WiFi");
    // We can use a partial update because Inkplate is in 1-bit mode
    display.partialUpdate();

    // Actually connect to the WiFi network
    WiFi.mode(WIFI_MODE_STA);
    WiFi.begin(ssid, pass);
    while (WiFi.status() != WL_CONNECTED)
    {
        // Print a dot every half second when connecting
        delay(500);
        display.print(".");
        display.partialUpdate();
        esp_task_wdt_reset(); // Keep watchdog happy during WiFi connection
    }
    
    Serial.println();
    Serial.print("Connected to WiFi network with IP Address: ");
    Serial.println(WiFi.localIP());
    Serial.println("Switching to 3-bit mode in 5 seconds...");
    display.partialUpdate();
    // Wait 5 seconds so that folks can see the IP if they want to for some reason.
    delay(5000);

    // Switch to 3-bit mode so the image will be of better quality
    // NOTE: You can't use partial update when the Inkplate is in the 3-bit mode!
    display.setDisplayMode(INKPLATE_3BIT);
    display.clearDisplay();
    display.display();
    getandprintdash();
}

void loop()
{
    // Reset watchdog timer
    esp_task_wdt_reset();

    // Every UPDATE_INTERVAL_IN_SECS seconds make the request
    if ((unsigned long)(millis() - lastConnectionTime) > UPDATE_INTERVAL_IN_SECS * 1000LL)
    {
      getandprintdash();
    }
    else {
      delay(1000);
    }
}

// Draw error overlay on the display (preserves existing image if hasLoadedImage).
// retryDelayMs: 0 means last attempt, show "Next retry at next refresh".
// errorDetail: short reason e.g. "Out of memory", "Decode failed"; NULL for HTTP/size errors.
// errorBody: HTTP error response body (NULL or empty when not applicable).
static void drawErrorOverlay(int attempt, unsigned long retryDelayMs, int lastHttpCode,
                            int32_t lastSize, const char *errorDetail, const char *errorBody,
                            bool hasLoadedImage) {
    if (!hasLoadedImage) {
        display.clearDisplay();
    }
    display.fillRect(0, 0, display.width(), ERROR_OVERLAY_HEIGHT, BLACK);
    // In 3 bit mode the text color for white is 7, in 1 bit mode you can use BLACK or WHITE.
    display.setTextColor(7);

    int lineY = 5;
    display.setCursor(5, lineY);
    display.print("Attempt ");
    display.print(attempt);
    display.print(" of ");
    display.print(MAX_RETRIES);
    display.println(" failed");

    lineY += 16;
    display.setCursor(5, lineY);
    if (errorDetail != NULL) {
        display.println(errorDetail);
    } else if (lastHttpCode != HTTP_CODE_OK) {
        display.print("HTTP ");
        display.println(lastHttpCode);
    } else {
        display.print("Invalid length: ");
        display.println(lastSize);
    }

    bool hasBody = (errorBody != NULL && errorBody[0] != '\0');
    if (hasBody) {
        // Show first ~55 chars of body, replacing newlines with spaces
        lineY += 16;
        display.setCursor(5, lineY);
        size_t bodyLen = strlen(errorBody);
        size_t limit = (bodyLen > 55) ? 55 : bodyLen;
        for (size_t i = 0; i < limit; i++) {
            char c = errorBody[i];
            display.print((c == '\n' || c == '\r') ? ' ' : c);
        }
        if (bodyLen > 55) display.print("...");
        display.println();
    }

    lineY += 16;
    display.setCursor(5, lineY);
    if (retryDelayMs > 0) {
        display.print("Retrying in ");
        display.print((unsigned long)(retryDelayMs / 1000));
        display.println(" s...");
    } else {
        display.print("Next retry at refresh (~");
        display.print(UPDATE_INTERVAL_IN_SECS / 60);
        display.println(" min)");
    }
    display.setTextColor(BLACK);
}

// Delay for ms milliseconds, resetting WDT periodically so long backoffs don't trigger the watchdog.
static void delayWithWdtReset(unsigned long ms) {
    unsigned long elapsed = 0;
    while (elapsed < ms) {
        unsigned long chunk = (ms - elapsed) > WDT_RESET_INTERVAL_MS ? WDT_RESET_INTERVAL_MS : (ms - elapsed);
        delay(chunk);
        elapsed += chunk;
        esp_task_wdt_reset();
    }
}

void getandprintdash() {
    bool success = false;
    int lastHttpCode = 0;
    int32_t lastSize = 0;
    const char *lastErrorDetail = NULL;       // Points to string literal, no heap alloc
    char lastErrorBody[MAX_ERROR_BODY_LEN + 1] = {0}; // Stack buffer for HTTP error body
    unsigned long retryDelay = RETRY_MIN_DELAY_MS;

    for (int attempt = 1; attempt <= MAX_RETRIES && !success; attempt++) {
        esp_task_wdt_reset(); // Feed WDT at start of each retry (before long GET)
        Serial.print("Attempt ");
        Serial.print(attempt);
        Serial.print(" of ");
        Serial.println(MAX_RETRIES);

        // Make an object for the HTTP client
        HTTPClient http;
        http.begin(url);
        http.setTimeout(60000);

        // Do a get request to get the image
        int httpCode = http.GET();
        esp_task_wdt_reset(); // Feed WDT after GET returns (GET can take up to 60s)
        lastHttpCode = httpCode;

        // If everything is OK
        if (httpCode == HTTP_CODE_OK)
        {
            // getSize() returns -1 when Content-Length is missing (e.g. chunked encoding)
            int32_t size = http.getSize();
            lastSize = size;
            if (size < 0) {
                Serial.println("Invalid response: Content-Length missing or invalid");
                lastErrorDetail = "Content-Length missing";
            } else if (size == 0) {
                Serial.println("Invalid response length: 0");
                lastErrorDetail = "Empty response";
            } else if ((uint32_t)size > MAX_IMAGE_SIZE) {
                Serial.print("Response too large: ");
                Serial.print(size);
                Serial.print(" (max ");
                Serial.print(MAX_IMAGE_SIZE);
                Serial.println(")");
                lastErrorDetail = "Response too large";
            } else {
                int32_t len = size; // Copy whose value we will change, but the original must not be lost

                // Allocate the memory for the image
                uint8_t *buffer = (uint8_t *)ps_malloc(size);

                if (buffer == NULL) {
                    Serial.println("Failed to allocate memory for image");
                    lastErrorDetail = "Out of memory";
                } else {

                uint8_t *buffPtr = buffer; // Copy of the buffer pointer so that the original one is not lost

                // Temporary buffer for retrieving parts of the image and storing them in the real buffer
                uint8_t buff[512] = {0};

                // Let's fetch the data
                WiFiClient *stream = http.getStreamPtr(); // We need a stream pointer to know how much data is available

                // Repeat as long as we have a connection and while there is data to read
                while (http.connected() && (len > 0 || len == -1))
                {
                    // Never read past the end of buffer (server may send more than Content-Length)
                    if (buffPtr >= buffer + size)
                        break;
                    size_t remaining = (size_t)((buffer + size) - buffPtr);
                    if (remaining == 0)
                        break;

                    size_t availableBytes = stream->available();
                    if (availableBytes)
                    {
                        size_t toRead = (availableBytes > sizeof(buff)) ? sizeof(buff) : availableBytes;
                        if (toRead > remaining)
                            toRead = remaining;
                        int c = stream->readBytes(buff, toRead);
                        if (c > 0) {
                            if ((size_t)c > remaining)
                                c = (int)remaining;
                            memcpy(buffPtr, buff, (size_t)c);
                            buffPtr += c;
                            if (len > 0)
                                len -= c;
                        }
                        esp_task_wdt_reset();
                    }
                    else if (len == -1)
                    {
                        len = 0;
                    }
                }

                // Clear buffer before draw: drawJpegFromBuffer only writes the decoded
                // rectangle, so old content would show in any area not covered by the JPEG.
                display.clearDisplay();
                // Draw image into the frame buffer; free buffer immediately after so we
                // never leak on draw failure or future code changes.
                bool drew = display.drawJpegFromBuffer(buffer, size, 0, 0, true, false);
                free(buffer);
                if (drew) {
                    success = true;
                    hasLoadedImage = true; // Mark that we've successfully loaded an image
                    Serial.println("Image loaded successfully");
                } else {
                    Serial.println("Failed to decode image");
                    lastErrorDetail = "Decode failed";
                }
                }
            }
        }
        else
        {
            Serial.print("HTTP error: ");
            Serial.println(httpCode);
            // Read error response body (capped) into stack buffer; log and show on overlay
            lastErrorBody[0] = '\0';
            WiFiClient *stream = http.getStreamPtr();
            if (stream) {
                uint8_t buf[64];
                size_t total = 0;
                while (total < MAX_ERROR_BODY_LEN && stream->available()) {
                    int n = stream->readBytes(buf, sizeof(buf));
                    if (n <= 0) break;
                    for (int i = 0; i < n && total < MAX_ERROR_BODY_LEN; i++) {
                        uint8_t c = buf[i];
                        if (c >= 32 && c < 127) lastErrorBody[total] = (char)c;
                        else if (c == '\n' || c == '\r' || c == '\t') lastErrorBody[total] = (char)c;
                        else lastErrorBody[total] = '?';
                        total++;
                    }
                    esp_task_wdt_reset();
                }
                lastErrorBody[total] = '\0';
                if (total > 0) {
                    Serial.print("Response body: ");
                    Serial.println(lastErrorBody);
                }
            }
        }

        http.end();

        // Show error overlay after each failed attempt, then wait before retry or exit
        if (!success) {
            unsigned long showDelayMs = (attempt < MAX_RETRIES) ? retryDelay : 0;
            drawErrorOverlay(attempt, showDelayMs, lastHttpCode, lastSize, lastErrorDetail, lastErrorBody, hasLoadedImage);
            display.display();
            if (attempt < MAX_RETRIES) {
                Serial.print("Retrying in ");
                Serial.print(retryDelay / 1000);
                Serial.println(" seconds...");
                delayWithWdtReset(retryDelay);
                retryDelay = min(retryDelay * RETRY_BACKOFF_MULTIPLIER, RETRY_MAX_DELAY_MS);
            }
        }
    }

    // Draw image on the screen
    display.display();
    // Don't clear buffer here: after success the buffer has the image (so next error overlay
    // draws on top of it), and after error it has the image+overlay (preserved for display).
    lastConnectionTime = millis();
}
