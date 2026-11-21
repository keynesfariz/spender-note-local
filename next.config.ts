import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  experimental: { goRoutes: true },
  allowedDevOrigins: ['127.0.0.1'],
  serverExternalPackages: ['node-llama-cpp'],
};

export default nextConfig;
