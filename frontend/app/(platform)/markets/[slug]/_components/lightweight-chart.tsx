/**
 * @description
 * This client component encapsulates the TradingView Lightweight Charts library.
 * It provides a professional-grade charting solution with custom news event markers.
 *
 * Key features:
 * - Client-Side Rendering: Explicitly marked as a client component (`'use client'`)
 *   because the charting library directly interacts with the DOM.
 * - Historical Data Loading: Fetches historical OHLCV data from our backend API.
 * - Real-time Updates: Subscribes to WebSocket data via Zustand store for live price updates.
 * - News Event Markers: Displays news events as custom markers on the chart with tooltips.
 * - Dark Theme: Configured to match the application's dark theme design system.
 * - Lifecycle Management: Properly cleans up chart resources when component unmounts.
 *
 * @dependencies
 * - react: For `useEffect`, `useRef`, and component logic.
 * - lightweight-charts: The TradingView Lightweight Charts library.
 * - @/lib/lw-chart-datafeed: The datafeed adapter for fetching and updating data.
 * - @/types: Contains the `NewsEvent` type definition.
 *
 * @notes
 * - This implementation uses TradingView Lightweight Charts, which is free and open-source.
 * - News markers are implemented as custom markers using the library's marker API.
 * - The component handles data fetching, real-time updates, and marker rendering.
 */

'use client'

import React, { useEffect, useRef, useState } from 'react'
import {
  createChart,
  IChartApi,
  ISeriesApi,
  CandlestickData,
  Time,
  MarkerPosition,
  MarkerShape,
  CandlestickSeries,
} from 'lightweight-charts'
import {
  fetchHistoricalData,
  subscribeToRealtimeUpdates,
  unsubscribeFromRealtimeUpdates,
  BarData,
} from '@/lib/lw-chart-datafeed'
import { NewsEvent } from '@/types'

interface LightweightChartProps {
  marketId: string
  newsEvents?: NewsEvent[]
}

const LightweightChart: React.FC<LightweightChartProps> = ({
  marketId,
  newsEvents = [],
}) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const markersRef = useRef<any[]>([])
  const subscriptionIdRef = useRef<string | null>(null)
  const mountedRef = useRef<boolean>(true)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Default resolution (15 minutes)
  const resolution = '15'

  useEffect(() => {
    if (!containerRef.current) return

    // Mark component as mounted
    mountedRef.current = true

    // Store the cleanup function
    let cleanup: (() => void) | undefined

    // Ensure container has dimensions before creating chart
    const container = containerRef.current
    const initChart = () => {
      if (!mountedRef.current || !containerRef.current) return
      cleanup = initializeChart()
    }

    if (container.clientWidth === 0 || container.clientHeight === 0) {
      // Wait for next frame to ensure container is rendered
      requestAnimationFrame(initChart)
    } else {
      initChart()
    }

    function initializeChart() {
      if (!containerRef.current || !mountedRef.current) return

      // Clean up any existing chart first
      if (chartRef.current) {
        try {
          chartRef.current.remove()
        } catch (e) {
          // Chart may already be disposed
        }
        chartRef.current = null
      }
      if (subscriptionIdRef.current) {
        unsubscribeFromRealtimeUpdates(marketId)
        subscriptionIdRef.current = null
      }

      // Initialize the chart
      const chart = createChart(containerRef.current!, {
        layout: {
          background: { color: '#0D1117' },
          textColor: '#C9D1D9',
        },
        grid: {
          vertLines: { color: '#161B22' },
          horzLines: { color: '#161B22' },
        },
        width: containerRef.current.clientWidth,
        height: 400,
        timeScale: {
          timeVisible: true,
          secondsVisible: false,
        },
        rightPriceScale: {
          borderColor: '#161B22',
        },
      })

      // Verify chart was created successfully
      if (!chart) {
        console.error('Chart initialization failed - chart is null')
        if (mountedRef.current) {
          setError('Failed to initialize chart')
          setIsLoading(false)
        }
        return
      }

      chartRef.current = chart

      // Create candlestick series
      // Try addCandlestickSeries first (convenience method), fall back to addSeries if not available
      let candlestickSeries: ISeriesApi<'Candlestick'>
      
      if (typeof (chart as any).addCandlestickSeries === 'function') {
        // Use convenience method if available
        candlestickSeries = (chart as any).addCandlestickSeries({
          upColor: '#3FB950',
          downColor: '#F85149',
          borderVisible: false,
          wickUpColor: '#3FB950',
          wickDownColor: '#F85149',
        })
      } else if (typeof chart.addSeries === 'function') {
        // Use addSeries with CandlestickSeries definition
        candlestickSeries = chart.addSeries(CandlestickSeries, {
          upColor: '#3FB950',
          downColor: '#F85149',
          borderVisible: false,
          wickUpColor: '#3FB950',
          wickDownColor: '#F85149',
        })
      } else {
        // Get all methods from prototype chain for debugging
        const chartProto = Object.getPrototypeOf(chart)
        const allMethods = Object.getOwnPropertyNames(chartProto).filter(
          (name) => typeof chart[name as keyof typeof chart] === 'function'
        )
        const seriesMethods = allMethods.filter((name) => name.includes('Series'))
        
        console.error('Neither addCandlestickSeries nor addSeries available on chart object', {
          chartType: typeof chart,
          chartConstructor: chart.constructor?.name,
          hasAddCandlestickSeries: 'addCandlestickSeries' in chart,
          hasAddSeries: 'addSeries' in chart,
          seriesMethods: seriesMethods,
          allMethods: allMethods.slice(0, 20), // First 20 methods
        })
        if (mountedRef.current) {
          setError('Chart API not available')
          setIsLoading(false)
        }
        return
      }

      seriesRef.current = candlestickSeries

      // Load historical data
      const loadHistoricalData = async () => {
        try {
          // Use refs to check validity instead of local variables
          if (!mountedRef.current || !chartRef.current || !seriesRef.current) {
            // Component unmounted or chart disposed, skip loading
            return
          }

          if (mountedRef.current) {
            setIsLoading(true)
            setError(null)
          }

          // Calculate time range (last 30 days)
          const to = Math.floor(Date.now() / 1000) // Current time in seconds
          const from = to - 30 * 24 * 60 * 60 // 30 days ago

          const bars = await fetchHistoricalData(
            marketId,
            from,
            to,
            resolution
          )

          // Check again if component is still mounted and chart is valid
          if (!mountedRef.current || !chartRef.current || !seriesRef.current) {
            return
          }

          let lastBar: BarData | null = null

          try {
            if (bars.length > 0) {
              // Set data to the series
              seriesRef.current.setData(bars as CandlestickData[])
              lastBar = bars[bars.length - 1]
            } else {
              // No historical data - create an empty initial bar that will be updated by real-time data
              // This allows the chart to work even if the database is empty
              const currentTime = Math.floor(Date.now() / 1000) as Time
              lastBar = {
                time: currentTime,
                open: 0,
                high: 0,
                low: 0,
                close: 0,
                volume: 0,
              }
              // Set empty data to initialize the series
              seriesRef.current.setData([])
            }
          } catch (setDataError) {
            // Chart may have been disposed (e.g., in React Strict Mode)
            if (setDataError instanceof Error && setDataError.message.includes('disposed')) {
              console.debug('Chart disposed before setData completed:', setDataError)
              return
            }
            throw setDataError
          }

          // Check again before subscribing
          if (!mountedRef.current || !chartRef.current || !seriesRef.current) {
            return
          }

          // Subscribe to real-time updates (even if no historical data)
          // The subscription will update the chart as data comes in
          subscriptionIdRef.current = subscribeToRealtimeUpdates(
            marketId,
            seriesRef.current,
            lastBar
          )

          // Add news event markers
          addNewsMarkers(seriesRef.current, newsEvents)

          if (mountedRef.current) {
            setIsLoading(false)
          }
        } catch (err) {
          console.error('Failed to load historical data:', err)
          
          // Check if component is still mounted before trying to set state or subscribe
          if (!mountedRef.current || !chartRef.current || !seriesRef.current) {
            return
          }

          // Even if historical data fails, try to subscribe to real-time updates
          try {
            const currentTime = Math.floor(Date.now() / 1000) as Time
            const initialBar: BarData = {
              time: currentTime,
              open: 0,
              high: 0,
              low: 0,
              close: 0,
              volume: 0,
            }
            subscriptionIdRef.current = subscribeToRealtimeUpdates(
              marketId,
              seriesRef.current,
              initialBar
            )
            if (mountedRef.current) {
              setError('Failed to load historical data, but real-time updates are active')
              setIsLoading(false)
            }
          } catch (subscribeError) {
            // Chart may have been disposed
            if (subscribeError instanceof Error && subscribeError.message.includes('disposed')) {
              console.debug('Chart disposed before subscription:', subscribeError)
              return
            }
            if (mountedRef.current) {
              setIsLoading(false)
            }
          }
        }
      }

      loadHistoricalData()

      // Handle window resize
      const handleResize = () => {
        if (containerRef.current && chartRef.current) {
          try {
            chartRef.current.applyOptions({
              width: containerRef.current.clientWidth,
            })
          } catch (e) {
            // Chart may be disposed
          }
        }
      }

      window.addEventListener('resize', handleResize)

      // Store cleanup function for this chart instance
      const cleanup = () => {
        window.removeEventListener('resize', handleResize)
        if (subscriptionIdRef.current) {
          unsubscribeFromRealtimeUpdates(marketId)
          subscriptionIdRef.current = null
        }
        if (chartRef.current) {
          try {
            chartRef.current.remove()
          } catch (e) {
            // Chart may already be disposed
          }
          chartRef.current = null
        }
        seriesRef.current = null
      }

      // Return cleanup function to be called when component unmounts or dependencies change
      return cleanup
    }

    // Cleanup function for useEffect
    return () => {
      // Mark component as unmounted to prevent state updates
      mountedRef.current = false
      
      // Unsubscribe from real-time updates first
      if (subscriptionIdRef.current) {
        unsubscribeFromRealtimeUpdates(marketId)
        subscriptionIdRef.current = null
      }
      // The cleanup function from initializeChart() already handles chart removal
      // Only call it if it exists, and don't try to remove the chart again
      if (cleanup) {
        cleanup()
      }
      // Clear the chart reference
      chartRef.current = null
      seriesRef.current = null
    }
  }, [marketId, newsEvents]) // Re-initialize if marketId or newsEvents changes

  // Update news markers when newsEvents prop changes
  useEffect(() => {
    if (seriesRef.current && newsEvents.length > 0) {
      // Clear existing markers
      markersRef.current = []
      // Add new markers
      addNewsMarkers(seriesRef.current, newsEvents)
    }
  }, [newsEvents])

  /**
   * @function addNewsMarkers
   * @description Adds news event markers to the chart series.
   * @param {ISeriesApi} series - The chart series to add markers to.
   * @param {NewsEvent[]} events - Array of news events to display.
   */
  const addNewsMarkers = (
    series: ISeriesApi<'Candlestick'>,
    events: NewsEvent[]
  ) => {
    if (!events || events.length === 0) return

    const markers = events.map((event) => {
      // Convert publishedAt ISO string to timestamp
      const eventTime = new Date(event.publishedAt).getTime() // Get milliseconds
      const time = (eventTime / 1000) as Time // Convert to seconds for Time type

      return {
        time: time,
        position: 'belowBar' as MarkerPosition,
        color: '#58A6FF',
        shape: 'circle' as MarkerShape,
        size: 1,
        text: event.title.substring(0, 50) + (event.title.length > 50 ? '...' : ''), // Truncate long titles
      }
    })

    // Add markers to the series
    series.setMarkers(markers)
    markersRef.current = markers
  }

  return (
    <div className="relative h-[400px] w-full">
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center bg-background/80">
          <p className="text-secondary">Loading chart data...</p>
        </div>
      )}
      {error && (
        <div className="absolute inset-0 flex items-center justify-center bg-background/80">
          <p className="text-destructive">{error}</p>
        </div>
      )}
      <div ref={containerRef} className="h-full w-full" />
    </div>
  )
}

export default LightweightChart

