package com.omnidex.integration;

import java.util.List;
import java.util.Map;

public record JobDetails(Job job, List<Map<String, Object>> steps, List<Map<String, Object>> contexts) {}
